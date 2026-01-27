// Package authz provides a Zanzibar-style ACL authorization system with
// a clean abstraction layer allowing the authorization backend to be swapped
// without affecting business logic.
package authz

import (
	"context"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
)

// AuthzMiddleware provides domain-specific authorization methods.
// These translate business operations to authorization checks.
type AuthzMiddleware interface {
	// Room permissions
	CanViewRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error
	CanSendMessage(ctx context.Context, actor *commonv1.ContactLink, roomID string) error
	CanUpdateRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error
	CanDeleteRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error
	CanManageMembers(ctx context.Context, actor *commonv1.ContactLink, roomID string) error
	CanManageRoles(ctx context.Context, actor *commonv1.ContactLink, roomID string) error

	// Message permissions
	CanDeleteMessage(
		ctx context.Context,
		actor *commonv1.ContactLink,
		messageID string,
		senderProfileID string,
		roomID string,
	) error
	CanEditMessage(ctx context.Context, actor *commonv1.ContactLink, messageID string, senderProfileID string) error

	// Batch operations (for efficiency in hot paths)
	CanSendMessagesToRooms(ctx context.Context, actor *commonv1.ContactLink, roomIDs []string) (map[string]bool, error)

	// Tuple management (for subscription changes)
	AddRoomMember(ctx context.Context, roomID string, profileID string, role string) error
	RemoveRoomMember(ctx context.Context, roomID string, profileID string) error
	UpdateRoomMemberRole(ctx context.Context, roomID string, profileID string, oldRole, newRole string) error

	// Message tuple management
	SetMessageSender(ctx context.Context, messageID string, senderProfileID string, roomID string) error
}
