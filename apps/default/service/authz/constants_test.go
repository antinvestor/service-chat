package authz_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/stretchr/testify/assert"
)

func TestRoleToRelation(t *testing.T) {
	tests := []struct {
		role     string
		expected string
	}{
		{authz.RoleOwner, authz.RelationOwner},
		{authz.RoleAdmin, authz.RelationAdmin},
		{authz.RoleMember, authz.RelationMember},
		{authz.RoleGuest, authz.RelationViewer},
		{"unknown", authz.RelationMember}, // Default to member
		{"", authz.RelationMember},        // Empty defaults to member
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := authz.RoleToRelation(tt.role)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRelationToRole(t *testing.T) {
	tests := []struct {
		relation string
		expected string
	}{
		{authz.RelationOwner, authz.RoleOwner},
		{authz.RelationAdmin, authz.RoleAdmin},
		{authz.RelationMember, authz.RoleMember},
		{authz.RelationViewer, authz.RoleGuest},
		{"unknown", authz.RoleMember}, // Default to member
		{"", authz.RoleMember},        // Empty defaults to member
	}

	for _, tt := range tests {
		t.Run(tt.relation, func(t *testing.T) {
			result := authz.RelationToRole(tt.relation)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidRoles(t *testing.T) {
	roles := authz.ValidRoles()
	assert.Contains(t, roles, authz.RoleOwner)
	assert.Contains(t, roles, authz.RoleAdmin)
	assert.Contains(t, roles, authz.RoleMember)
	assert.Contains(t, roles, authz.RoleGuest)
	assert.Len(t, roles, 4)
}

func TestValidRelations(t *testing.T) {
	relations := authz.ValidRelations()
	assert.Contains(t, relations, authz.RelationOwner)
	assert.Contains(t, relations, authz.RelationAdmin)
	assert.Contains(t, relations, authz.RelationMember)
	assert.Contains(t, relations, authz.RelationViewer)
	assert.Len(t, relations, 4)
}

func TestIsValidRole(t *testing.T) {
	assert.True(t, authz.IsValidRole(authz.RoleOwner))
	assert.True(t, authz.IsValidRole(authz.RoleAdmin))
	assert.True(t, authz.IsValidRole(authz.RoleMember))
	assert.True(t, authz.IsValidRole(authz.RoleGuest))
	assert.False(t, authz.IsValidRole("invalid"))
	assert.False(t, authz.IsValidRole(""))
}

func TestConstants(t *testing.T) {
	// Verify namespace constants
	assert.Equal(t, "room", authz.NamespaceRoom)
	assert.Equal(t, "message", authz.NamespaceMessage)
	assert.Equal(t, "profile", authz.NamespaceProfile)

	// Verify relation constants
	assert.Equal(t, "owner", authz.RelationOwner)
	assert.Equal(t, "admin", authz.RelationAdmin)
	assert.Equal(t, "member", authz.RelationMember)
	assert.Equal(t, "viewer", authz.RelationViewer)
	assert.Equal(t, "sender", authz.RelationSender)
	assert.Equal(t, "room", authz.RelationRoom)

	// Verify permission constants
	assert.Equal(t, "view", authz.PermissionView)
	assert.Equal(t, "send_message", authz.PermissionSendMessage)
	assert.Equal(t, "delete_any_message", authz.PermissionDeleteAnyMessage)
	assert.Equal(t, "update", authz.PermissionUpdate)
	assert.Equal(t, "delete", authz.PermissionDelete)
	assert.Equal(t, "manage_members", authz.PermissionManageMembers)
	assert.Equal(t, "manage_roles", authz.PermissionManageRoles)
	assert.Equal(t, "edit", authz.PermissionEdit)
	assert.Equal(t, "react", authz.PermissionReact)

	// Verify role constants
	assert.Equal(t, "owner", authz.RoleOwner)
	assert.Equal(t, "admin", authz.RoleAdmin)
	assert.Equal(t, "member", authz.RoleMember)
	assert.Equal(t, "guest", authz.RoleGuest)
}
