package repository

import (
	"context"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
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

	// Role hierarchy levels (highest to lowest).
	roleOwnerLevel  = 4
	roleAdminLevel  = 3
	roleMemberLevel = 2
	roleGuestLevel  = 1
)

type roomSubscriptionRepository struct {
	datastore.BaseRepository[*models.RoomSubscription]

	activeSubscriptionStates []models.RoomSubscriptionState
}

// NewRoomSubscriptionRepository creates a new room subscription repository instance.
func NewRoomSubscriptionRepository(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
) RoomSubscriptionRepository {
	return &roomSubscriptionRepository{
		BaseRepository: datastore.NewBaseRepository[*models.RoomSubscription](
			ctx, dbPool, workMan, func() *models.RoomSubscription { return &models.RoomSubscription{} },
		),

		activeSubscriptionStates: []models.RoomSubscriptionState{
			models.RoomSubscriptionStateActive,
		},
	}
}

// GetByContactLinkAndRooms retrieves a subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetByContactLinkAndRooms(
	ctx context.Context, contactLink *commonv1.ContactLink,
	roomIDList ...string,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Pool().DB(ctx, true).
		Select("*"). // Explicitly select all columns including read-only unread_count
		Where("  room_id IN ? AND contact_link_id = ? ", roomIDList, contactLink.GetContactId())

	if contactLink.GetProfileId() != "" {
		query = query.Where(" profile_id = ? ", contactLink.GetProfileId())
	}

	err := query.Find(subscriptions).Error
	return subscriptions, err
}

// GetOneByRoomContactLinkAndIsActive retrieves an active subscription by room ID and profile ID.
func (rsr *roomSubscriptionRepository) GetOneByRoomContactLinkAndIsActive(ctx context.Context, roomID string, contactLink *commonv1.ContactLink) (*models.RoomSubscription, error) {
	subscription := &models.RoomSubscription{}
	query := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND  contact_link_id = ? ", roomID, contactLink.GetContactId())

	if contactLink.GetProfileId() != "" {
		query = query.Where(" profile_id = ? ", contactLink.GetProfileId())
	}

	err := query.First(subscription).Error
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

// GetByRoomIDAndContactLinks retrieves a subscription by room ID and a list of profile IDs.
func (rsr *roomSubscriptionRepository) GetByRoomIDAndContactLinks(ctx context.Context, roomID string, contactLink ...*commonv1.ContactLink) ([]*models.RoomSubscription, error) {
	var subscriptionSlice []*models.RoomSubscription

	profileIDList := make([]string, len(contactLink))
	contactIDList := make([]string, len(contactLink))

	for i, cl := range contactLink {
		if cl.GetProfileId() != "" {
			profileIDList[i] = cl.GetProfileId()
		}
		contactIDList[i] = cl.GetContactId()
	}

	err := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND ( profile_id IN ? OR contact_id IN ? )", roomID, profileIDList, contactIDList).
		Find(&subscriptionSlice).Error
	return subscriptionSlice, err
}

// GetByContactLink retrieves all subscriptions for a specific profile.
func (rsr *roomSubscriptionRepository) GetByContactLink(ctx context.Context, contactLink *commonv1.ContactLink,
	activeOnly bool,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Pool().DB(ctx, true).
		Preload(clause.Associations).
		Where(" contact_link_id = ? ", contactLink.GetContactId())

	if contactLink.GetProfileId() != "" {
		query = query.Where(" profile_id = ? ", contactLink.GetProfileId())
	}

	if activeOnly {
		query = query.Where("subscription_state IN ?", rsr.activeSubscriptionStates)
	}

	err := query.Find(&subscriptions).Error
	return subscriptions, err
}

// GetMembersByRoomID retrieves all active member profile IDs for a room.
func (rsr *roomSubscriptionRepository) GetMembersByRoomID(ctx context.Context, roomID string) ([]*commonv1.ContactLink, error) {
	var subscriptions []*models.RoomSubscription
	err := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND subscription_state IN ?", roomID, rsr.activeSubscriptionStates).Find(subscriptions).Error

	contactLinks := make([]*commonv1.ContactLink, len(subscriptions))
	for _, sub := range subscriptions {
		contactLinks = append(contactLinks, sub.ToLink())
	}

	return contactLinks, err
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
	_, err := rsr.BulkUpdate(
		ctx,
		subscriptionIDs,
		map[string]any{"subscription_state": models.RoomSubscriptionStateBlocked},
	)
	if err != nil {
		return err
	}
	return nil
}

// Activate marks a subscription as active.
func (rsr *roomSubscriptionRepository) Activate(ctx context.Context, subscriptionIDs ...string) error {
	_, err := rsr.BulkUpdate(
		ctx,
		subscriptionIDs,
		map[string]any{"subscription_state": models.RoomSubscriptionStateActive},
	)
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
	roomID string, contactLink *commonv1.ContactLink, minRole string,
) (bool, error) {
	subscription, err := rsr.GetOneByRoomContactLinkAndIsActive(ctx, roomID, contactLink)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, err
	}

	// Role hierarchy: owner > admin > member > guest
	roleHierarchy := map[string]int{
		RoleOwner:  roleOwnerLevel,
		RoleAdmin:  roleAdminLevel,
		RoleMember: roleMemberLevel,
		RoleGuest:  roleGuestLevel,
	}

	userRoleLevel := roleHierarchy[subscription.Role]
	minRoleLevel := roleHierarchy[minRole]

	return userRoleLevel >= minRoleLevel, nil
}

// IsActiveMember checks if a profile is an active member of a room.
func (rsr *roomSubscriptionRepository) IsActiveMember(ctx context.Context, roomID string, contactLink *commonv1.ContactLink) (bool, error) {
	subscription, err := rsr.GetOneByRoomContactLinkAndIsActive(ctx, roomID, contactLink)
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

// GetByRoomIDPaged retrieves active subscriptions for a room with keyset pagination.
func (rsr *roomSubscriptionRepository) GetByRoomIDPaged(
	ctx context.Context,
	roomID string,
	lastID string,
	limit int,
) ([]*models.RoomSubscription, error) {
	var subscriptions []*models.RoomSubscription
	query := rsr.Pool().DB(ctx, true).
		Where("room_id = ? AND subscription_state IN ?", roomID, rsr.activeSubscriptionStates)

	if lastID != "" {
		query = query.Where("id > ?", lastID)
	}

	err := query.Order("id ASC").Limit(limit).Find(&subscriptions).Error
	return subscriptions, err
}
