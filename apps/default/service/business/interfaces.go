package business

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
)

// RoomBusiness defines the business logic for room operations.
type RoomBusiness interface {
	// CreateRoom creates a new room with the given parameters and adds the creator as an admin
	CreateRoom(
		ctx context.Context,
		req *chatv1.CreateRoomRequest,
		createdBy *commonv1.ContactLink,
	) (*chatv1.Room, error)

	// GetRoom retrieves a room by ID with proper authorization checks
	GetRoom(ctx context.Context, roomID string, profileID string) (*chatv1.Room, error)

	// UpdateRoom updates room details (name, description, etc.)
	UpdateRoom(ctx context.Context, req *chatv1.UpdateRoomRequest, updatedBy string) (*chatv1.Room, error)

	// DeleteRoom soft deletes a room (only for admins/owners)
	DeleteRoom(ctx context.Context, req *chatv1.DeleteRoomRequest, profileID string) error

	// SearchRooms retrieves rooms based on filters and pagination
	SearchRooms(ctx context.Context, req *chatv1.SearchRoomsRequest, profileID string) ([]*chatv1.Room, error)

	// AddRoomSubscriptions adds members to a room with a specific role
	AddRoomSubscriptions(ctx context.Context, req *chatv1.AddRoomSubscriptionsRequest, addedBy string) error

	// RemoveRoomSubscriptions removes members from a room
	RemoveRoomSubscriptions(ctx context.Context, req *chatv1.RemoveRoomSubscriptionsRequest, removerID string) error

	// UpdateSubscriptionRole updates a member's role in a room
	UpdateSubscriptionRole(ctx context.Context, req *chatv1.UpdateSubscriptionRoleRequest, updaterID string) error

	// SearchRoomSubscriptions retrieves all members of a room with their roles
	SearchRoomSubscriptions(
		ctx context.Context,
		req *chatv1.SearchRoomSubscriptionsRequest,
		profileID string,
	) ([]*chatv1.RoomSubscription, error)
}

// MessageBusiness defines the business logic for message operations.
type MessageBusiness interface {
	// SendEvents sends an event to a room with proper validation and permissions
	SendEvents(ctx context.Context, req *chatv1.SendEventRequest, senderID string) ([]*chatv1.StreamAck, error)

	// GetHistory retrieves message history for a room with pagination
	GetHistory(ctx context.Context, req *chatv1.GetHistoryRequest, profileID string) ([]*chatv1.RoomEvent, error)

	// MarkMessagesAsRead updates the last read sequence for a user in a room
	MarkMessagesAsRead(ctx context.Context, roomID string, eventID string, profileID string) error
}

// ClientStateBusiness defines the business logic for real-time connection operations.
type ClientStateBusiness interface {

	// UpdatePresence sends presence updates to room subscribers
	UpdatePresence(ctx context.Context, status *chatv1.PresenceEvent) error

	// UpdateTypingIndicator sends typing indicators to room subscribers
	UpdateTypingIndicator(ctx context.Context, profileID string, roomID string, isTyping bool) error

	// UpdateReadReceipt sends read receipts to room subscribers
	UpdateReadReceipt(ctx context.Context, profileID string, roomID string, eventID string) error

	// UpdateReadMarker updates the read marker to room subscribers
	UpdateReadMarker(ctx context.Context, profileID string, roomID string, eventID string) error
}
