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
	assert.ElementsMatch(t, []string{authz.RoleOwner, authz.RoleAdmin, authz.RoleMember, authz.RoleGuest}, roles)
}

func TestValidRelations(t *testing.T) {
	relations := authz.ValidRelations()
	assert.ElementsMatch(t, []string{authz.RelationOwner, authz.RelationAdmin, authz.RelationMember, authz.RelationViewer}, relations)
}

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{authz.RoleOwner, true},
		{authz.RoleAdmin, true},
		{authz.RoleMember, true},
		{authz.RoleGuest, true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			assert.Equal(t, tt.expected, authz.IsValidRole(tt.role))
		})
	}
}

func TestConstants(t *testing.T) {
	testCases := []struct {
		name     string
		actual   string
		expected string
	}{
		{"NamespaceRoom", authz.NamespaceRoom, "room"},
		{"NamespaceMessage", authz.NamespaceMessage, "message"},
		{"NamespaceProfile", authz.NamespaceProfile, "profile"},
		{"RelationOwner", authz.RelationOwner, "owner"},
		{"RelationAdmin", authz.RelationAdmin, "admin"},
		{"RelationMember", authz.RelationMember, "member"},
		{"RelationViewer", authz.RelationViewer, "viewer"},
		{"RelationSender", authz.RelationSender, "sender"},
		{"RelationRoom", authz.RelationRoom, "room"},
		{"PermissionView", authz.PermissionView, "view"},
		{"PermissionSendMessage", authz.PermissionSendMessage, "send_message"},
		{"PermissionDeleteAnyMessage", authz.PermissionDeleteAnyMessage, "delete_any_message"},
		{"PermissionUpdate", authz.PermissionUpdate, "update"},
		{"PermissionDelete", authz.PermissionDelete, "delete"},
		{"PermissionManageMembers", authz.PermissionManageMembers, "manage_members"},
		{"PermissionManageRoles", authz.PermissionManageRoles, "manage_roles"},
		{"PermissionEdit", authz.PermissionEdit, "edit"},
		{"PermissionReact", authz.PermissionReact, "react"},
		{"RoleOwner", authz.RoleOwner, "owner"},
		{"RoleAdmin", authz.RoleAdmin, "admin"},
		{"RoleMember", authz.RoleMember, "member"},
		{"RoleGuest", authz.RoleGuest, "guest"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.actual)
		})
	}
}
