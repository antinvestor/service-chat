package authz

import "slices"

// Namespace constants define the object types in the authorization system.
const (
	NamespaceRoom    = "room"
	NamespaceMessage = "message"
	NamespaceProfile = "profile"
)

// Relation constants define the relationships between objects and subjects.
const (
	RelationOwner  = "owner"
	RelationAdmin  = "admin"
	RelationMember = "member"
	RelationViewer = "viewer"
	RelationSender = "sender"
	RelationRoom   = "room"
)

// Permission constants define the actions that can be performed on objects.
const (
	PermissionView             = "view"
	PermissionSendMessage      = "send_message"
	PermissionDeleteAnyMessage = "delete_any_message"
	PermissionUpdate           = "update"
	PermissionDelete           = "delete"
	PermissionManageMembers    = "manage_members"
	PermissionManageRoles      = "manage_roles"
	PermissionEdit             = "edit"
	PermissionReact            = "react"
)

// Role constants define the member roles in a room.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
	RoleGuest  = "guest"
)

// RoleToRelation converts a role string to a Keto relation.
func RoleToRelation(role string) string {
	switch role {
	case RoleOwner:
		return RelationOwner
	case RoleAdmin:
		return RelationAdmin
	case RoleMember:
		return RelationMember
	case RoleGuest:
		return RelationViewer
	default:
		return RelationMember
	}
}

// RelationToRole converts a Keto relation to a role string.
func RelationToRole(relation string) string {
	switch relation {
	case RelationOwner:
		return RoleOwner
	case RelationAdmin:
		return RoleAdmin
	case RelationMember:
		return RoleMember
	case RelationViewer:
		return RoleGuest
	default:
		return RoleMember
	}
}

// ValidRoles returns a list of valid role strings.
func ValidRoles() []string {
	return []string{RoleOwner, RoleAdmin, RoleMember, RoleGuest}
}

// ValidRelations returns a list of valid relation strings for rooms.
func ValidRelations() []string {
	return []string{RelationOwner, RelationAdmin, RelationMember, RelationViewer}
}

// IsValidRole checks if a role string is valid.
func IsValidRole(role string) bool {
	return slices.Contains(ValidRoles(), role)
}
