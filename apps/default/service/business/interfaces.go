package business

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/models"
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
	GetRoom(ctx context.Context, roomID string, searchedBy *commonv1.ContactLink) (*chatv1.Room, error)

	// UpdateRoom updates room details (name, description, etc.)
	UpdateRoom(
		ctx context.Context,
		req *chatv1.UpdateRoomRequest,
		updatedBy *commonv1.ContactLink,
	) (*chatv1.Room, error)

	// DeleteRoom soft deletes a room (only for admins/owners)
	DeleteRoom(ctx context.Context, req *chatv1.DeleteRoomRequest, deletedBy *commonv1.ContactLink) error

	// SearchRooms retrieves rooms based on filters and pagination
	SearchRooms(
		ctx context.Context,
		req *chatv1.SearchRoomsRequest,
		searchedBy *commonv1.ContactLink,
	) ([]*chatv1.Room, error)

	// AddRoomSubscriptions adds members to a room with a specific role
	AddRoomSubscriptions(
		ctx context.Context,
		req *chatv1.AddRoomSubscriptionsRequest,
		addedBy *commonv1.ContactLink,
	) error

	// RemoveRoomSubscriptions removes members from a room
	RemoveRoomSubscriptions(
		ctx context.Context,
		req *chatv1.RemoveRoomSubscriptionsRequest,
		removedBy *commonv1.ContactLink,
	) error

	// UpdateSubscriptionRole updates a member's role in a room
	UpdateSubscriptionRole(
		ctx context.Context,
		req *chatv1.UpdateSubscriptionRoleRequest,
		updatedBy *commonv1.ContactLink,
	) error

	// SearchRoomSubscriptions retrieves all members of a room with their roles
	SearchRoomSubscriptions(
		ctx context.Context,
		req *chatv1.SearchRoomSubscriptionsRequest,
		searchedBy *commonv1.ContactLink,
	) ([]*chatv1.RoomSubscription, error)
}

// MessageBusiness defines the business logic for message operations.
type MessageBusiness interface {
	// SendEvents sends an event to a room with proper validation and permissions
	SendEvents(
		ctx context.Context,
		req *chatv1.SendEventRequest,
		sentBy *commonv1.ContactLink,
	) ([]*chatv1.AckEvent, error)

	// GetMessage retrieves a single message by ID with access control
	GetMessage(
		ctx context.Context,
		messageID string,
		gottenBy *commonv1.ContactLink,
	) (*models.RoomEvent, error)

	// GetHistory retrieves message history for a room with pagination
	GetHistory(
		ctx context.Context,
		req *chatv1.GetHistoryRequest,
		gottenBy *commonv1.ContactLink,
	) ([]*chatv1.RoomEvent, error)

	// DeleteMessage deletes a message (sender or admin/owner only)
	DeleteMessage(ctx context.Context, messageID string, deletedBy *commonv1.ContactLink) error

	// MarkMessagesAsRead updates the last read sequence for a user in a room
	MarkMessagesAsRead(ctx context.Context, roomID string, eventID string, markedBy *commonv1.ContactLink) error
}

// ProposalManagement defines the business logic for a proposal approval/voting workflow.
// Implementations are scoped to a specific entity type (e.g., rooms) but the interface
// is generic enough to be reused for other voting processes.
type ProposalManagement interface {
	// Approve approves a pending proposal and executes the proposed change.
	Approve(
		ctx context.Context,
		scopeID string,
		proposalID string,
		approvedBy *commonv1.ContactLink,
	) error

	// Reject rejects a pending proposal.
	Reject(
		ctx context.Context,
		scopeID string,
		proposalID string,
		reason string,
		rejectedBy *commonv1.ContactLink,
	) error

	// ListPending retrieves all pending proposals for a given scope.
	ListPending(
		ctx context.Context,
		scopeID string,
		searchedBy *commonv1.ContactLink,
	) ([]*models.Proposal, error)
}

// ClientStateBusiness defines the business logic for real-time connection operations.
type ClientStateBusiness interface {

	// UpdatePresence sends presence updates to room subscribers
	UpdatePresence(ctx context.Context, status *chatv1.PresenceEvent) error

	// UpdateTypingIndicator sends typing indicators to room subscribers
	UpdateTypingIndicator(ctx context.Context, roomID string, reader *commonv1.ContactLink, isTyping bool) error

	// UpdateReadReceipt sends read receipts to room subscribers
	UpdateDeliveryReceipt(ctx context.Context, roomID string, reader *commonv1.ContactLink, eventID ...string) error

	// UpdateReadMarker updates the read marker to room subscribers
	UpdateReadMarker(ctx context.Context, roomID string, reader *commonv1.ContactLink, eventID string) error
}
