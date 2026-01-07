package business

import (
	"context"
	"errors"
	"fmt"
	"strings"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame"
)

type roleLevel int

const (
	roleOwnerLevel  roleLevel = 3
	roleAdminLevel  roleLevel = 2
	roleMemberLevel roleLevel = 1
	roleGuestLevel  roleLevel = 0

	roleOwner  = "owner"
	roleAdmin  = "admin"
	roleMember = "member"
	roleGuest  = "guest"
)

// SubscriptionService defines the interface for subscription-related operations.
type SubscriptionService interface {
	// HasAccess checks if a user has access to a room
	HasAccess(
		ctx context.Context,
		contact *commonv1.ContactLink,
		roomID ...string,
	) ([]*models.RoomSubscription, error)

	// HasRole checks if a user has a specific role in a room
	HasRole(
		ctx context.Context,
		contact *commonv1.ContactLink,
		roomID string,
		roleLvl roleLevel,
	) (*models.RoomSubscription, error)

	// CanManageMembers checks if a user can add/remove members from a room
	CanManageMembers(
		ctx context.Context,
		contact *commonv1.ContactLink,
		roomID string,
	) (*models.RoomSubscription, error)

	// CanManageRoles checks if a user can change member roles in a room
	CanManageRoles(ctx context.Context, contact *commonv1.ContactLink, roomID string) (*models.RoomSubscription, error)

	// GetSubscribedRoomIDs returns all room IDs a user is subscribed to
	GetSubscribedRoomIDs(ctx context.Context, contact *commonv1.ContactLink) ([]string, error)
}

type subscriptionService struct {
	service       *frame.Service
	subRepo       repository.RoomSubscriptionRepository
	roleHierarchy map[string]roleLevel
}

// NewSubscriptionService creates a new subscription service.
func NewSubscriptionService(
	service *frame.Service,
	subRepo repository.RoomSubscriptionRepository,
) SubscriptionService {
	return &subscriptionService{
		service: service,
		subRepo: subRepo,

		// Define role hierarchy (highest to lowest)
		roleHierarchy: map[string]roleLevel{
			roleOwner:  roleOwnerLevel,
			roleAdmin:  roleAdminLevel,
			roleMember: roleMemberLevel,
			roleGuest:  roleGuestLevel,
		},
	}
}

func (ss *subscriptionService) HasAccess(
	ctx context.Context,
	contact *commonv1.ContactLink,
	roomID ...string,
) ([]*models.RoomSubscription, error) {
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

	// If no subscriptions found, deny access
	if len(subscriptionList) == 0 {
		return nil, service.ErrRoomAccessDenied
	}

	return subscriptionList, nil
}

func (ss *subscriptionService) HasRole(
	ctx context.Context,
	contact *commonv1.ContactLink,
	roomID string, roleLevel roleLevel) (*models.RoomSubscription, error) {
	if err := internal.IsValidContactLink(contact); err != nil {
		return nil, err
	}

	if roomID == "" {
		return nil, service.ErrMessageRoomIDRequired
	}

	subscriptionList, err := ss.subRepo.GetByContactLinkAndRooms(ctx, contact, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check subscription: %w", err)
	}

	for _, sub := range subscriptionList {
		if !sub.IsActive() {
			continue // Skip inactive subscriptions
		}

		// Check if the user has the required role or higher
		if ss.hasMinimumRole(roleLevel, strings.Split(sub.Role, ",")...) {
			return sub, nil
		}
	}

	return nil, errors.New("no subscription found")
}

func (ss *subscriptionService) CanManageMembers(
	ctx context.Context,
	contact *commonv1.ContactLink,
	roomID string,
) (*models.RoomSubscription, error) {
	// Admins and owners can manage members
	manager, err := ss.HasRole(ctx, contact, roomID, roleAdminLevel)
	if err != nil {
		return nil, err
	}

	return manager, nil
}

func (ss *subscriptionService) CanManageRoles(
	ctx context.Context,
	contact *commonv1.ContactLink,
	roomID string,
) (*models.RoomSubscription, error) {
	// Only owners can manage roles
	return ss.HasRole(ctx, contact, roomID, roleOwnerLevel)
}

func (ss *subscriptionService) GetSubscribedRoomIDs(
	ctx context.Context,
	contact *commonv1.ContactLink,
) ([]string, error) {
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

// hasMinimumRole checks if the user's role meets or exceeds the required role.
func (ss *subscriptionService) hasMinimumRole(requiredRoleLevel roleLevel, userRoleList ...string) bool {
	for _, role := range userRoleList {
		// Default to lowest privilege if role is unknown
		userLevel, ok := ss.roleHierarchy[role]
		if !ok {
			continue
		}

		if userLevel >= requiredRoleLevel {
			return true
		}
	}

	return false
}
