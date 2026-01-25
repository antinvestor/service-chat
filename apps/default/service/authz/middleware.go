package authz

import (
	"context"
	"fmt"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
)

// authzMiddleware implements the AuthzMiddleware interface.
type authzMiddleware struct {
	service     AuthzService
	auditLogger AuditLogger
}

// NewAuthzMiddleware creates a new AuthzMiddleware with the given service and audit logger.
func NewAuthzMiddleware(service AuthzService, auditLogger AuditLogger) AuthzMiddleware {
	return &authzMiddleware{
		service:     service,
		auditLogger: auditLogger,
	}
}

// CanViewRoom checks if the actor can view a room.
func (m *authzMiddleware) CanViewRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionView)
}

// CanSendMessage checks if the actor can send messages to a room.
func (m *authzMiddleware) CanSendMessage(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionSendMessage)
}

// CanUpdateRoom checks if the actor can update a room.
func (m *authzMiddleware) CanUpdateRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionUpdate)
}

// CanDeleteRoom checks if the actor can delete a room.
func (m *authzMiddleware) CanDeleteRoom(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionDelete)
}

// CanManageMembers checks if the actor can add/remove members from a room.
func (m *authzMiddleware) CanManageMembers(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionManageMembers)
}

// CanManageRoles checks if the actor can change member roles in a room.
func (m *authzMiddleware) CanManageRoles(ctx context.Context, actor *commonv1.ContactLink, roomID string) error {
	return m.checkRoomPermission(ctx, actor, roomID, PermissionManageRoles)
}

// CanDeleteMessage checks if the actor can delete a message.
// Fast path: sender can always delete their own message.
func (m *authzMiddleware) CanDeleteMessage(ctx context.Context, actor *commonv1.ContactLink, messageID, senderProfileID, roomID string) error {
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
func (m *authzMiddleware) CanEditMessage(ctx context.Context, actor *commonv1.ContactLink, messageID, senderProfileID string) error {
	profileID := actor.GetProfileId()

	// Only sender can edit their own message
	if profileID == senderProfileID {
		return nil
	}

	return NewPermissionDeniedError(
		ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		PermissionEdit,
		SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		"only the message sender can edit the message",
	)
}

// CanSendMessagesToRooms checks if the actor can send messages to multiple rooms.
// Returns a map of room ID to allowed status.
func (m *authzMiddleware) CanSendMessagesToRooms(ctx context.Context, actor *commonv1.ContactLink, roomIDs []string) (map[string]bool, error) {
	if len(roomIDs) == 0 {
		return map[string]bool{}, nil
	}

	profileID := actor.GetProfileId()
	requests := make([]CheckRequest, len(roomIDs))
	for i, roomID := range roomIDs {
		requests[i] = CheckRequest{
			Object:     ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Permission: PermissionSendMessage,
			Subject:    SubjectRef{Namespace: NamespaceProfile, ID: profileID},
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
func (m *authzMiddleware) AddRoomMember(ctx context.Context, roomID, profileID, role string) error {
	relation := RoleToRelation(role)
	return m.service.WriteTuple(ctx, RelationTuple{
		Object:   ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Relation: relation,
		Subject:  SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	})
}

// RemoveRoomMember removes all relations for a member from a room.
func (m *authzMiddleware) RemoveRoomMember(ctx context.Context, roomID, profileID string) error {
	// Remove all relations for this member
	tuples := make([]RelationTuple, len(ValidRelations()))
	for i, rel := range ValidRelations() {
		tuples[i] = RelationTuple{
			Object:   ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Relation: rel,
			Subject:  SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		}
	}
	return m.service.DeleteTuples(ctx, tuples)
}

// UpdateRoomMemberRole updates a member's role in a room.
func (m *authzMiddleware) UpdateRoomMemberRole(ctx context.Context, roomID, profileID, oldRole, newRole string) error {
	// Remove old relation if specified
	if oldRole != "" {
		_ = m.service.DeleteTuple(ctx, RelationTuple{
			Object:   ObjectRef{Namespace: NamespaceRoom, ID: roomID},
			Relation: RoleToRelation(oldRole),
			Subject:  SubjectRef{Namespace: NamespaceProfile, ID: profileID},
		})
	}

	// Add new relation
	return m.service.WriteTuple(ctx, RelationTuple{
		Object:   ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Relation: RoleToRelation(newRole),
		Subject:  SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	})
}

// SetMessageSender creates a sender relation for a message.
func (m *authzMiddleware) SetMessageSender(ctx context.Context, messageID, senderProfileID, roomID string) error {
	// Create sender relation
	senderTuple := RelationTuple{
		Object:   ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		Relation: RelationSender,
		Subject:  SubjectRef{Namespace: NamespaceProfile, ID: senderProfileID},
	}

	// Create room relation
	roomTuple := RelationTuple{
		Object:   ObjectRef{Namespace: NamespaceMessage, ID: messageID},
		Relation: RelationRoom,
		Subject:  SubjectRef{Namespace: NamespaceRoom, ID: roomID},
	}

	return m.service.WriteTuples(ctx, []RelationTuple{senderTuple, roomTuple})
}

// checkRoomPermission is a helper that checks a room permission and returns an appropriate error.
func (m *authzMiddleware) checkRoomPermission(ctx context.Context, actor *commonv1.ContactLink, roomID, permission string) error {
	profileID := actor.GetProfileId()
	if profileID == "" {
		return ErrInvalidSubject
	}

	req := CheckRequest{
		Object:     ObjectRef{Namespace: NamespaceRoom, ID: roomID},
		Permission: permission,
		Subject:    SubjectRef{Namespace: NamespaceProfile, ID: profileID},
	}

	result, err := m.service.Check(ctx, req)
	if err != nil {
		return fmt.Errorf("authorization check failed: %w", err)
	}

	if !result.Allowed {
		return NewPermissionDeniedError(
			req.Object,
			permission,
			req.Subject,
			result.Reason,
		)
	}

	return nil
}
