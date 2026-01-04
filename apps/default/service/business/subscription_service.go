package business

import (
	"context"
	"fmt"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame"
)

// SubscriptionService defines the interface for subscription-related operations.
type SubscriptionService interface {
	// HasAccess checks if a user has access to a room
	HasAccess(ctx context.Context, contact *commonv1.ContactLink, roomID ...string) (map[*models.RoomSubscription]bool, error)

	// HasRole checks if a user has a specific role in a room
	HasRole(ctx context.Context, contact *commonv1.ContactLink, roomID, role string) (bool, error)

	// CanManageMembers checks if a user can add/remove members from a room
	CanManageMembers(ctx context.Context, contact *commonv1.ContactLink, roomID string) (bool, error)

	// CanManageRoles checks if a user can change member roles in a room
	CanManageRoles(ctx context.Context, contact *commonv1.ContactLink, roomID string) (bool, error)

	// GetSubscribedRoomIDs returns all room IDs a user is subscribed to
	GetSubscribedRoomIDs(ctx context.Context, contact *commonv1.ContactLink) ([]string, error)
}

type subscriptionService struct {
	service *frame.Service
	subRepo repository.RoomSubscriptionRepository
}

// NewSubscriptionService creates a new subscription service.
func NewSubscriptionService(
	service *frame.Service,
	subRepo repository.RoomSubscriptionRepository,
) SubscriptionService {
	return &subscriptionService{
		service: service,
		subRepo: subRepo,
	}
}

func (ss *subscriptionService) HasAccess(ctx context.Context, contact *commonv1.ContactLink, roomID ...string) (map[*models.RoomSubscription]bool, error) {
	if err := internal.IsValidContactLink(contact); err != nil {
		return nil, err
	}

	if len(roomID) == 0 {
		return nil, service.ErrMessageRoomIDRequired
	}

	subscriptionList, err := ss.subRepo.GetByContactLinkAndRooms(ctx, contact, roomID...)
	if err != nil {
		return nil, fmt.Errorf("failed to check subscription: %w", err)
	}

	subscMap := make(map[*models.RoomSubscription]bool)
	for _, s := range subscriptionList {
		subscMap[s] = s.IsActive()
	}

	return subscMap, nil
}

func (ss *subscriptionService) HasRole(ctx context.Context, contact *commonv1.ContactLink, roomID, role string) (bool, error) {
	if err := internal.IsValidContactLink(contact); err != nil {
		return false, err
	}

	if roomID == "" {
		return false, service.ErrMessageRoomIDRequired
	}

	if role == "" {
		return false, service.ErrRoleRequired
	}

	subscriptionList, err := ss.subRepo.GetByContactLinkAndRooms(ctx, contact, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to check subscription: %w", err)
	}

	for _, sub := range subscriptionList {
		if !sub.IsActive() {
			return false, nil
		}

		// Check if the user has the required role or higher
		return hasMinimumRole(sub.Role, role), nil
	}

	return false, fmt.Errorf("no subscription found")

}

func (ss *subscriptionService) CanManageMembers(ctx context.Context, contact *commonv1.ContactLink, roomID string) (bool, error) {
	// Admins and owners can manage members
	isAdmin, err := ss.HasRole(ctx, contact, roomID, "admin")
	if err != nil {
		return false, err
	}

	isOwner, err := ss.HasRole(ctx, contact, roomID, "owner")
	if err != nil {
		return false, err
	}

	return isAdmin || isOwner, nil
}

func (ss *subscriptionService) CanManageRoles(ctx context.Context, contact *commonv1.ContactLink, roomID string) (bool, error) {
	// Only owners can manage roles
	return ss.HasRole(ctx, contact, roomID, "owner")
}

func (ss *subscriptionService) GetSubscribedRoomIDs(ctx context.Context, contact *commonv1.ContactLink) ([]string, error) {
	if err := internal.IsValidContactLink(contact); err != nil {
		return nil, err
	}

	subscriptions, err := ss.subRepo.GetByContactLink(ctx, contact, true) // active only
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	roomIDs := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		roomIDs = append(roomIDs, sub.RoomID)
	}

	return roomIDs, nil
}

const (
	roleOwnerLevel  = 3
	roleAdminLevel  = 2
	roleMemberLevel = 1
	roleGuestLevel  = 0
)

// hasMinimumRole checks if the user's role meets or exceeds the required role.
func hasMinimumRole(userRole, requiredRole string) bool {
	// Define role hierarchy (highest to lowest)
	roleHierarchy := map[string]int{
		"owner":  roleOwnerLevel,
		"admin":  roleAdminLevel,
		"member": roleMemberLevel,
		"guest":  roleGuestLevel,
	}

	// Default to lowest privilege if role is unknown
	userLevel, ok := roleHierarchy[userRole]
	if !ok {
		userLevel = -1
	}

	requiredLevel, ok := roleHierarchy[requiredRole]
	if !ok {
		// If required role is unknown, assume it's a high privilege
		return false
	}

	return userLevel >= requiredLevel
}
