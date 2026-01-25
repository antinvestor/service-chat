// Package authz provides a Zanzibar-style ACL authorization system with
// a clean abstraction layer allowing the authorization backend to be swapped
// without affecting business logic.
package authz

import (
	"context"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
)

// ObjectRef represents a reference to an object (resource).
type ObjectRef struct {
	Namespace string // "room", "message", "profile"
	ID        string // Object identifier
}

// SubjectRef represents a reference to a subject (actor).
type SubjectRef struct {
	Namespace string // Usually "profile"
	ID        string // Profile ID
	Relation  string // Optional: for subject sets (e.g., "room:123#member")
}

// RelationTuple represents a relationship between object and subject.
type RelationTuple struct {
	Object   ObjectRef
	Relation string // "owner", "admin", "member", "sender", etc.
	Subject  SubjectRef
}

// CheckRequest represents a permission check request.
type CheckRequest struct {
	Object     ObjectRef
	Permission string // "view", "delete", "send_message", etc.
	Subject    SubjectRef
}

// CheckResult represents the result of a permission check.
type CheckResult struct {
	Allowed   bool
	Reason    string // Explanation for audit
	CheckedAt int64  // Unix timestamp
}

// AuthzService is the core authorization service interface.
// Implementations can be swapped without affecting business logic.
type AuthzService interface {
	// Check verifies if a subject has permission on an object.
	Check(ctx context.Context, req CheckRequest) (CheckResult, error)

	// BatchCheck verifies multiple permissions in one call (for efficiency).
	BatchCheck(ctx context.Context, requests []CheckRequest) ([]CheckResult, error)

	// WriteTuple creates a relationship tuple.
	WriteTuple(ctx context.Context, tuple RelationTuple) error

	// WriteTuples creates multiple relationship tuples atomically.
	WriteTuples(ctx context.Context, tuples []RelationTuple) error

	// DeleteTuple removes a relationship tuple.
	DeleteTuple(ctx context.Context, tuple RelationTuple) error

	// DeleteTuples removes multiple relationship tuples atomically.
	DeleteTuples(ctx context.Context, tuples []RelationTuple) error

	// ListRelations returns all relations for an object.
	ListRelations(ctx context.Context, object ObjectRef) ([]RelationTuple, error)

	// ListSubjectRelations returns all objects a subject has relations to.
	ListSubjectRelations(ctx context.Context, subject SubjectRef, namespace string) ([]RelationTuple, error)

	// Expand returns all subjects with a given relation (for member listing).
	Expand(ctx context.Context, object ObjectRef, relation string) ([]SubjectRef, error)
}

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
	CanDeleteMessage(ctx context.Context, actor *commonv1.ContactLink, messageID string, senderProfileID string, roomID string) error
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

// AuditLogger logs authorization decisions for security audit.
type AuditLogger interface {
	LogDecision(ctx context.Context, req CheckRequest, result CheckResult, metadata map[string]string) error
}
