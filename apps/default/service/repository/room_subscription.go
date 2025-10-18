package repository

import (
	"context"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/framedata"
	"gorm.io/gorm/clause"

	"github.com/antinvestor/service-chat/apps/default/service/models"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleGuest  = "guest"
)

type roomSubscriptionRepository struct {
	framedata.BaseRepository[*models.RoomSubscription]
}

// GetByID retrieves a room subscription by its ID.
func (rsr *roomSubscriptionRepository) GetByID(ctx context.Context, id string) (*models.RoomSubscription, error) {
	subscription := &models.RoomSubscription{}
	err := rsr.Svc().DB(ctx, true).First(subscription, "id = ?", id).Error
	return subscription, err
}

// Save creates or updates a room subscription.
func (rsr *roomSubscriptionRepository) Save(ctx context.Context, subscription *models.RoomSubscription) error {
	return rsr.Svc().DB(ctx, false).Save(subscription).Error
}

// Delete soft deletes a room subscription by its ID.
func (rsr *roomSubscriptionRepository) Delete(ctx context.Context, id string) error {
	subscription, err := rsr.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return rsr.Svc().DB(ctx, false).Delete(subscription).Error
}

// GetByRoomAndProfile retrieves a subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetByRoomAndProfile(
	ctx context.Context,
	roomID, profileID string,
) (*models.RoomSubscription, error) {
	subscription := &models.RoomSubscription{}
	err := rsr.Svc().DB(ctx, true).
		Where("room_id = ? AND profile_id = ?", roomID, profileID).
		First(subscription).Error
	return subscription, err
}

// GetActiveByRoomAndProfile retrieves an active subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetActiveByRoomAndProfile(
	ctx context.Context,
	roomID, profileID string,
) (*models.RoomSubscription, error) {
	subscription := &models.RoomSubscription{}
	err := rsr.Svc().DB(ctx, true).
		Where("room_id = ? AND profile_id = ? AND is_active = ?", roomID, profileID, true).
		First(subscription).Error
	return subscription, err
}

// GetByRoomID retrieves all subscriptions for a specific room.
func (rsr *roomSubscriptionRepository) GetByRoomID(ctx context.Context, roomID string, activeOnly bool) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Svc().DB(ctx, true).Where("room_id = ?", roomID)

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	err := query.Find(&subscriptions).Error
	return subscriptions, err
}

// GetByProfileID retrieves all subscriptions for a specific profile.
func (rsr *roomSubscriptionRepository) GetByProfileID(ctx context.Context, profileID string, activeOnly bool) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Svc().DB(ctx, true).
		Preload(clause.Associations).
		Where("profile_id = ?", profileID)

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	err := query.Find(&subscriptions).Error
	return subscriptions, err
}

// GetMembersByRoomID retrieves all active member profile IDs for a room.
func (rsr *roomSubscriptionRepository) GetMembersByRoomID(ctx context.Context, roomID string) ([]string, error) {
	var profileIDs []string
	err := rsr.Svc().DB(ctx, true).
		Model(&models.RoomSubscription{}).
		Where("room_id = ? AND is_active = ?", roomID, true).
		Pluck("profile_id", &profileIDs).Error
	return profileIDs, err
}

// GetByRole retrieves subscriptions by room ID and role.
func (rsr *roomSubscriptionRepository) GetByRole(ctx context.Context, roomID, role string) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	err := rsr.Svc().DB(ctx, true).
		Where("room_id = ? AND role = ? AND is_active = ?", roomID, role, true).
		Find(&subscriptions).Error
	return subscriptions, err
}

// UpdateRole updates the role of a subscription.
func (rsr *roomSubscriptionRepository) UpdateRole(ctx context.Context, id, role string) error {
	return rsr.Svc().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("role", role).Error
}

// UpdateLastReadSequence updates the last read sequence for a subscription.
func (rsr *roomSubscriptionRepository) UpdateLastReadSequence(ctx context.Context, id string, sequence int64) error {
	return rsr.Svc().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("last_read_sequence", sequence).Error
}

// Deactivate marks a subscription as inactive.
func (rsr *roomSubscriptionRepository) Deactivate(ctx context.Context, id string) error {
	return rsr.Svc().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("is_active", false).Error
}

// Activate marks a subscription as active.
func (rsr *roomSubscriptionRepository) Activate(ctx context.Context, id string) error {
	return rsr.Svc().DB(ctx, false).
		Model(&models.RoomSubscription{}).
		Where("id = ?", id).
		Update("is_active", true).Error
}

// CountActiveMembers counts the number of active members in a room.
func (rsr *roomSubscriptionRepository) CountActiveMembers(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rsr.Svc().DB(ctx, true).
		Model(&models.RoomSubscription{}).
		Where("room_id = ? AND is_active = ?", roomID, true).
		Count(&count).Error
	return count, err
}

// HasPermission checks if a profile has a specific role or higher in a room.
func (rsr *roomSubscriptionRepository) HasPermission(ctx context.Context, roomID, profileID, minRole string) (bool, error) {
	subscription, err := rsr.GetActiveByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if frame.ErrorIsNoRows(err) {
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
	subscription, err := rsr.GetActiveByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if frame.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, err
	}
	return subscription.IsActive, nil
}

// BulkCreate creates multiple subscriptions in a single transaction.
func (rsr *roomSubscriptionRepository) BulkCreate(ctx context.Context, subscriptions []*models.RoomSubscription) error {
	return rsr.Svc().DB(ctx, false).Create(&subscriptions).Error
}

// NewRoomSubscriptionRepository creates a new room subscription repository instance.
func NewRoomSubscriptionRepository(service *frame.Service) RoomSubscriptionRepository {
	return &roomSubscriptionRepository{
		BaseRepository: framedata.NewBaseRepository[*models.RoomSubscription](service, func() *models.RoomSubscription { return &models.RoomSubscription{} }),
	}
}
