package business

import (
	"context"
	"fmt"

	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
)

// SubscriptionService defines the interface for subscription-related operations
type SubscriptionService interface {
	// HasAccess checks if a user has access to a room
	HasAccess(ctx context.Context, profileID, roomID string) (bool, error)

	// HasRole checks if a user has a specific role in a room
	HasRole(ctx context.Context, profileID, roomID, role string) (bool, error)

	// CanManageMembers checks if a user can add/remove members from a room
	CanManageMembers(ctx context.Context, profileID, roomID string) (bool, error)

	// CanManageRoles checks if a user can change member roles in a room
	CanManageRoles(ctx context.Context, profileID, roomID string) (bool, error)

	// GetSubscribedRoomIDs returns all room IDs a user is subscribed to
	GetSubscribedRoomIDs(ctx context.Context, profileID string) ([]string, error)
}

type subscriptionService struct {
	service *frame.Service
	subRepo repository.RoomSubscriptionRepository
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(
	service *frame.Service,
	subRepo repository.RoomSubscriptionRepository,
) SubscriptionService {
	return &subscriptionService{
		service: service,
		subRepo: subRepo,
	}
}

func (ss *subscriptionService) HasAccess(ctx context.Context, profileID, roomID string) (bool, error) {
	if profileID == "" {
		return false, service.ErrProfileIDsRequired
	}

	if roomID == "" {
		return false, service.ErrMessageRoomIDRequired
	}

	sub, err := ss.subRepo.GetByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if frame.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check subscription: %w", err)
	}

	return sub.IsActive, nil
}

func (ss *subscriptionService) HasRole(ctx context.Context, profileID, roomID, role string) (bool, error) {
	if profileID == "" {
		return false, service.ErrProfileIDsRequired
	}

	if roomID == "" {
		return false, service.ErrMessageRoomIDRequired
	}

	if role == "" {
		return false, service.ErrRoleRequired
	}

	sub, err := ss.subRepo.GetByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if frame.ErrorIsNoRows(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check subscription: %w", err)
	}

	if !sub.IsActive {
		return false, nil
	}

	// Check if the user has the required role or higher
	return hasMinimumRole(sub.Role, role), nil
}

func (ss *subscriptionService) CanManageMembers(ctx context.Context, profileID, roomID string) (bool, error) {
	// Admins and owners can manage members
	isAdmin, err := ss.HasRole(ctx, profileID, roomID, "admin")
	if err != nil {
		return false, err
	}

	isOwner, err := ss.HasRole(ctx, profileID, roomID, "owner")
	if err != nil {
		return false, err
	}

	return isAdmin || isOwner, nil
}

func (ss *subscriptionService) CanManageRoles(ctx context.Context, profileID, roomID string) (bool, error) {
	// Only owners can manage roles
	return ss.HasRole(ctx, profileID, roomID, "owner")
}

func (ss *subscriptionService) GetSubscribedRoomIDs(ctx context.Context, profileID string) ([]string, error) {
	if profileID == "" {
		return nil, service.ErrProfileIDsRequired
	}

	subscriptions, err := ss.subRepo.GetByProfileID(ctx, profileID, true) // active only
	if err != nil {
		return nil, fmt.Errorf("failed to get user subscriptions: %w", err)
	}

	roomIDs := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		roomIDs = append(roomIDs, sub.RoomID)
	}

	return roomIDs, nil
}

// hasMinimumRole checks if the user's role meets or exceeds the required role
func hasMinimumRole(userRole, requiredRole string) bool {
	// Define role hierarchy (highest to lowest)
	roleHierarchy := map[string]int{
		"owner":  3,
		"admin":  2,
		"member": 1,
		"guest":  0,
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
