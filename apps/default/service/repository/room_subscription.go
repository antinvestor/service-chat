package repository

import (
	"context"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
	"gorm.io/gorm/clause"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleGuest  = "guest"
)

type roomSubscriptionRepository struct {
	datastore.BaseRepository[*models.RoomSubscription]

	activeSubscriptionStates []models.RoomSubscriptionState
}

// GetOneByRoomAndProfile retrieves a subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetOneByRoomAndProfile(
	ctx context.Context,
	roomID, profileID string,
) (*models.RoomSubscription, error) {
	subscription := &models.RoomSubscription{}
	err := rsr.Pool().DB(ctx, true).
		Select("*"). // Explicitly select all columns including read-only unread_count
		Where("room_id = ? AND profile_id = ?", roomID, profileID).
		First(subscription).Error
	return subscription, err
}

// GetOneByRoomProfileAndIsActive retrieves an active subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetOneByRoomProfileAndIsActive(
	ctx context.Context,
	roomID string, profileID string,
) (*models.RoomSubscription, error) {

	subscription := &models.RoomSubscription{}
	err := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND profile_id = ? AND subscription_state IN ?", roomID, profileID, rsr.activeSubscriptionStates).
		First(subscription).Error
	return subscription, err
}

// GetByRoomID retrieves all subscriptions for a specific room.
func (rsr *roomSubscriptionRepository) GetByRoomID(
	ctx context.Context,
	roomID string,
	activeOnly bool,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Pool().DB(ctx, true).Where("room_id = ?", roomID)

	if activeOnly {
		query = query.Where("subscription_state IN ?", rsr.activeSubscriptionStates)
	}

	err := query.Find(&subscriptions).Error
	return subscriptions, err
}

// GetByRoomIDAndProfiles retrieves a subscription by room ID and a list of profile IDs.
func (rsr *roomSubscriptionRepository) GetByRoomIDAndProfiles(ctx context.Context, roomID string, profileID ...string) ([]*models.RoomSubscription, error) {
	var subscriptionSlice []*models.RoomSubscription
	err := rsr.Pool().DB(ctx, true).
		Select("*"). // Explicitly select all columns including read-only unread_count
		Where("room_id = ? AND profile_id = ?", roomID, profileID).
		Find(&subscriptionSlice).Error
	return subscriptionSlice, err
}

// GetByProfileID retrieves all subscriptions for a specific profile.
func (rsr *roomSubscriptionRepository) GetByProfileID(
	ctx context.Context,
	profileID string,
	activeOnly bool,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Pool().DB(ctx, true).
		Preload(clause.Associations).
		Where("profile_id = ?", profileID)

	if activeOnly {
		query = query.Where("subscription_state IN ?", rsr.activeSubscriptionStates)
	}

	err := query.Find(&subscriptions).Error
	return subscriptions, err
}

// GetMembersByRoomID retrieves all active member profile IDs for a room.
func (rsr *roomSubscriptionRepository) GetMembersByRoomID(ctx context.Context, roomID string) ([]string, error) {
	var profileIDs []string
	err := rsr.Pool().DB(ctx, true).
		Model(&models.RoomSubscription{}).
		Where("room_id = ? AND subscription_state IN ?", roomID, rsr.activeSubscriptionStates).
		Pluck("profile_id", &profileIDs).Error
	return profileIDs, err
}

// GetByRole retrieves subscriptions by room ID and role.
func (rsr *roomSubscriptionRepository) GetByRole(
	ctx context.Context,
	roomID, role string,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	err := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND role = ? AND subscription_state IN ?", roomID, role, rsr.activeSubscriptionStates).
		Find(&subscriptions).Error
	return subscriptions, err
}

// UpdateRole updates the role of a subscription.
func (rsr *roomSubscriptionRepository) UpdateRole(ctx context.Context, id, role string) error {
	return rsr.Pool().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("role", role).Error
}

// UpdateLastReadEventID updates the last read event ID for a subscription.
func (rsr *roomSubscriptionRepository) UpdateLastReadEventID(ctx context.Context, id string, eventID string) error {
	return rsr.Pool().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("last_read_event_id", eventID).Error
}

// Deactivate marks a subscription as inactive.
func (rsr *roomSubscriptionRepository) Deactivate(ctx context.Context, subscriptionIDs ...string) error {

	_, err := rsr.BulkUpdate(ctx, subscriptionIDs, map[string]any{"subscription_state": models.RoomSubscriptionStateBlocked})
	if err != nil {
		return err
	}
	return nil
}

// Activate marks a subscription as active.
func (rsr *roomSubscriptionRepository) Activate(ctx context.Context, subscriptionIDs ...string) error {

	_, err := rsr.BulkUpdate(ctx, subscriptionIDs, map[string]any{"subscription_state": models.RoomSubscriptionStateActive})
	if err != nil {
		return err
	}
	return nil
}

// CountActiveMembers counts the number of active members in a room.
func (rsr *roomSubscriptionRepository) CountActiveMembers(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rsr.Pool().DB(ctx, true).
		Model(&models.RoomSubscription{}).
		Where("room_id = ? AND subscription_state IN ?", roomID, rsr.activeSubscriptionStates).
		Count(&count).Error
	return count, err
}

// HasPermission checks if a profile has a specific role or higher in a room.
func (rsr *roomSubscriptionRepository) HasPermission(
	ctx context.Context,
	roomID, profileID, minRole string,
) (bool, error) {
	subscription, err := rsr.GetOneByRoomProfileAndIsActive(ctx, roomID, profileID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, err
	}

	// Role hierarchy: owner > admin > member > guest
	roleHierarchy := map[string]int{
		RoleOwner:  4,
		RoleAdmin:  3,
		RoleMember: 2,
		RoleGuest:  1,
	}

	userRoleLevel := roleHierarchy[subscription.Role]
	minRoleLevel := roleHierarchy[minRole]

	return userRoleLevel >= minRoleLevel, nil
}

// IsActiveMember checks if a profile is an active member of a room.
func (rsr *roomSubscriptionRepository) IsActiveMember(ctx context.Context, roomID, profileID string) (bool, error) {
	subscription, err := rsr.GetOneByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, err
	}
	return subscription.IsActive(), nil
}

// BulkCreate creates multiple subscriptions in a single transaction.
func (rsr *roomSubscriptionRepository) BulkCreate(ctx context.Context, subscriptions []*models.RoomSubscription) error {
	return rsr.Pool().DB(ctx, false).Create(&subscriptions).Error
}

// NewRoomSubscriptionRepository creates a new room subscription repository instance.
func NewRoomSubscriptionRepository(ctx context.Context, dbPool pool.Pool, workMan workerpool.Manager) RoomSubscriptionRepository {
	return &roomSubscriptionRepository{
		BaseRepository: datastore.NewBaseRepository[*models.RoomSubscription](
			ctx, dbPool, workMan, func() *models.RoomSubscription { return &models.RoomSubscription{} },
		),

		activeSubscriptionStates: []models.RoomSubscriptionState{
			models.RoomSubscriptionStateActive,
		},
	}
}
