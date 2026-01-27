package authz

import (
	"context"
	"fmt"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
)

// middleware implements the Middleware interface.
type middleware struct {
	service security.Authorizer
}

// NewMiddleware creates a new Middleware with the given authorizer service.
func NewMiddleware(service security.Authorizer) Middleware {
	return &middleware{
		service: service,
	}
}

// CanViewRoom checks if the actor can view a room.
func (m *middleware) CanViewRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionView)
}

// CanSendMessage checks if the actor can send messages to a room.
func (m *middleware) CanSendMessage(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionSendMessage)
}

// CanUpdateRoom checks if the actor can update a room.
func (m *middleware) CanUpdateRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionUpdate)
}

// CanDeleteRoom checks if the actor can delete a room.
func (m *middleware) CanDeleteRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionDelete)
}

// CanManageMembers checks if the actor can add/remove members from a room.
func (m *middleware) CanManageMembers(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionManageMembers)
}

// CanManageRoles checks if the actor can change member roles in a room.
func (m *middleware) CanManageRoles(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionManageRoles)
}

// CanDeleteMessage checks if the actor can delete a message.
// Fast path: sender can always delete their own message.
func (m *middleware) CanDeleteMessage(
	ctx context.Context,
	actor *commonv1.ContactLink,
	_, senderProfileID, roomID string,
) error {
	profileID := actor.GetProfileId()

	// Fast path: sender can always delete their own message
	if profileID == senderProfileID {
		return nil
	}

	// Check room admin/owner permission for deleting others' messages
	return m.checkRoomPermission(ctx, actor, roomID, PermissionDeleteAnyMessage)
}

// CanEditMessage checks if the actor can edit a message.
// Only the sender can edit their own message.
func (m *middleware) CanEditMessage(
	_ context.Context,
	actor *commonv1.ContactLink,
	messageID, senderProfileID string,
) error {
	profileID := actor.GetProfileId()

	// Only sender can edit their own message
	if profileID == senderProfileID {
		return nil
	}

	return authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		PermissionEdit,
		security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		"only the message sender can edit the message",
	)
}

// CanSendMessagesToRooms checks if the actor can send messages to multiple rooms.
// Returns a map of room ID to allowed status.
func (m *middleware) CanSendMessagesToRooms(
	ctx context.Context,
	actor *commonv1.ContactLink,
	roomIDs []string,
) (map[string]bool, error) {
	if len(roomIDs) == 0 {
		return map[string]bool{}, nil
	}

	profileID := actor.GetProfileId()
	requests := make([]security.CheckRequest, len(roomIDs))
	for i, roomID := range roomIDs {
		requests[i] = security.CheckRequest{
			Object:     security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Permission: PermissionSendMessage,
			Subject:    security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		}
	}

	results, err := m.service.BatchCheck(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("batch check failed: %w", err)
	}

	allowed := make(map[string]bool, len(roomIDs))
	for i, roomID := range roomIDs {
		allowed[roomID] = results[i].Allowed
	}
	return allowed, nil
}

// AddRoomMember adds a member to a room with the specified role.
func (m *middleware) AddRoomMember(ctx context.Context, roomID, profileID, role string) error {
	relation := RoleToRelation(role)
	return m.service.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Relation: relation,
		Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	})
}

// RemoveRoomMember removes all relations for a member from a room.
func (m *middleware) RemoveRoomMember(ctx context.Context, roomID, profileID string) error {
	// Remove all relations for this member
	tuples := make([]security.RelationTuple, len(ValidRelations()))
	for i, rel := range ValidRelations() {
		tuples[i] = security.RelationTuple{
			Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Relation: rel,
			Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		}
	}
	return m.service.DeleteTuples(ctx, tuples)
}

// UpdateRoomMemberRole updates a member's role in a room.
func (m *middleware) UpdateRoomMemberRole(ctx context.Context, roomID, profileID, oldRole, newRole string) error {
	// Remove old relation if specified
	if oldRole != "" {
		_ = m.service.DeleteTuple(ctx, security.RelationTuple{
			Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Relation: RoleToRelation(oldRole),
			Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		})
	}

	// Add new relation
	return m.service.WriteTuple(ctx, security.RelationTuple{
		Object:   security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Relation: RoleToRelation(newRole),
		Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	})
}

// SetMessageSender creates a sender relation for a message.
func (m *middleware) SetMessageSender(ctx context.Context, messageID, senderProfileID, roomID string) error {
	// Create sender relation
	senderTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		Relation: RelationSender,
		Subject:  security.SubjectRef{Namespace: NamespaceProfile, ID: senderProfileID},
	}

	// Create room relation
	roomTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		Relation: RelationRoom,
		Subject:  security.SubjectRef{Namespace: NamespaceRoom, ID: roomID},
	}

	return m.service.WriteTuples(ctx, []security.RelationTuple{senderTuple, roomTuple})
}

// checkRoomPermission is a helper that checks a room permission and returns an appropriate error.
func (m *middleware) checkRoomPermission(
	ctx context.Context,
	actor *commonv1.ContactLink,
	roomID, permission string,
) error {
	profileID := actor.GetProfileId()
	if profileID == "" {
		return authorizer.ErrInvalidSubject
	}

	req := security.CheckRequest{
		Object:     security.ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Permission: permission,
		Subject:    security.SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	}

	result, err := m.service.Check(ctx, req)
	if err != nil {
		return fmt.Errorf("authorization check failed: %w", err)
	}

	if !result.Allowed {
		return authorizer.NewPermissionDeniedError(
			req.Object,
			permission,
			req.Subject,
			result.Reason,
		)
	}

	return nil
}
