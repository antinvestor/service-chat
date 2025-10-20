package business

import (
	"context"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
)

// RoomBusiness defines the business logic for room operations.
type RoomBusiness interface {
	// CreateRoom creates a new room with the given parameters and adds the creator as an admin
	CreateRoom(ctx context.Context, req *chatv1.CreateRoomRequest, createdBy string) (*chatv1.Room, error)

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

// ConnectBusiness defines the business logic for real-time connection operations.
type ConnectBusiness interface {
	// HandleConnection manages a bidirectional streaming connection for real-time events
	HandleConnection(ctx context.Context, profileID string, deviceID string, stream ConnectionStream) error

	// ReceiveEvent receives an event from a connected user
	ReceiveEvent(ctx context.Context, event *chatv1.RoomEvent) (error, *chatv1.StreamAck)

	// BroadcastEvent sends an event to all subscribers of a room
	BroadcastEvent(ctx context.Context, roomID string, event *chatv1.ServerEvent) error

	// SendPresenceUpdate sends presence updates to room subscribers
	SendPresenceUpdate(ctx context.Context, profileID string, roomID string, status chatv1.PresenceStatus) error

	// SendTypingIndicator sends typing indicators to room subscribers
	SendTypingIndicator(ctx context.Context, profileID string, roomID string, isTyping bool) error

	// SendReadReceipt sends read receipts to room subscribers
	SendReadReceipt(ctx context.Context, profileID string, roomID string, eventID string) error
}

// ConnectionStream abstracts the bidirectional stream for testing.
type ConnectionStream interface {
	Receive() (*chatv1.ConnectRequest, error)
	Send(*chatv1.ServerEvent) error
}
