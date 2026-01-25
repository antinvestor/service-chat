// Package mock provides a mock implementation of authz.AuthzService for testing.
package mock

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/authz"
)

// MockAuthzService is an in-memory implementation of authz.AuthzService for testing.
type MockAuthzService struct {
	mu     sync.RWMutex
	tuples map[string]authz.RelationTuple

	// AllowAll when true, all checks return allowed.
	AllowAll bool

	// CheckFunc allows custom check logic for testing.
	CheckFunc func(ctx context.Context, req authz.CheckRequest) (authz.CheckResult, error)
}

// NewMockAuthzService creates a new mock authorization service.
func NewMockAuthzService() *MockAuthzService {
	return &MockAuthzService{
		tuples: make(map[string]authz.RelationTuple),
	}
}

// tupleKey generates a unique key for a relation tuple.
func tupleKey(t authz.RelationTuple) string {
	key := t.Object.Namespace + ":" + t.Object.ID + "#" + t.Relation + "@"
	if t.Subject.Relation != "" {
		key += t.Subject.Namespace + ":" + t.Subject.ID + "#" + t.Subject.Relation
	} else {
		key += t.Subject.Namespace + ":" + t.Subject.ID
	}
	return key
}

// Check verifies if a subject has permission on an object.
func (m *MockAuthzService) Check(ctx context.Context, req authz.CheckRequest) (authz.CheckResult, error) {
	// Allow custom check function for testing
	if m.CheckFunc != nil {
		return m.CheckFunc(ctx, req)
	}

	if m.AllowAll {
		return authz.CheckResult{
			Allowed:   true,
			Reason:    "mock: all allowed",
			CheckedAt: time.Now().Unix(),
		}, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for direct relation
	allowed := m.hasRelation(req.Object, req.Permission, req.Subject)

	// Check permission mappings (simplified for mock)
	if !allowed {
		allowed = m.checkPermissionHierarchy(req)
	}

	return authz.CheckResult{
		Allowed:   allowed,
		CheckedAt: time.Now().Unix(),
	}, nil
}

// hasRelation checks if a specific relation exists.
func (m *MockAuthzService) hasRelation(object authz.ObjectRef, relation string, subject authz.SubjectRef) bool {
	tuple := authz.RelationTuple{
		Object:   object,
		Relation: relation,
		Subject:  subject,
	}
	_, exists := m.tuples[tupleKey(tuple)]
	return exists
}

// checkPermissionHierarchy implements simplified permission inheritance for testing.
func (m *MockAuthzService) checkPermissionHierarchy(req authz.CheckRequest) bool {
	// Room permission hierarchy
	if req.Object.Namespace == authz.NamespaceRoom {
		switch req.Permission {
		case authz.PermissionView:
			// viewer, member, admin, owner can view
			return m.hasRelation(req.Object, authz.RelationViewer, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationMember, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationAdmin, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationOwner, req.Subject)

		case authz.PermissionSendMessage:
			// member, admin, owner can send
			return m.hasRelation(req.Object, authz.RelationMember, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationAdmin, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationOwner, req.Subject)

		case authz.PermissionDeleteAnyMessage, authz.PermissionUpdate, authz.PermissionManageMembers:
			// admin, owner can delete any message, update, manage members
			return m.hasRelation(req.Object, authz.RelationAdmin, req.Subject) ||
				m.hasRelation(req.Object, authz.RelationOwner, req.Subject)

		case authz.PermissionDelete, authz.PermissionManageRoles:
			// only owner can delete room or manage roles
			return m.hasRelation(req.Object, authz.RelationOwner, req.Subject)
		}
	}

	return false
}

// BatchCheck verifies multiple permissions in one call.
func (m *MockAuthzService) BatchCheck(ctx context.Context, requests []authz.CheckRequest) ([]authz.CheckResult, error) {
	results := make([]authz.CheckResult, len(requests))
	for i, req := range requests {
		result, err := m.Check(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// WriteTuple creates a relationship tuple.
func (m *MockAuthzService) WriteTuple(ctx context.Context, tuple authz.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tuples[tupleKey(tuple)] = tuple
	return nil
}

// WriteTuples creates multiple relationship tuples atomically.
func (m *MockAuthzService) WriteTuples(ctx context.Context, tuples []authz.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tuple := range tuples {
		m.tuples[tupleKey(tuple)] = tuple
	}
	return nil
}

// DeleteTuple removes a relationship tuple.
func (m *MockAuthzService) DeleteTuple(ctx context.Context, tuple authz.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tuples, tupleKey(tuple))
	return nil
}

// DeleteTuples removes multiple relationship tuples atomically.
func (m *MockAuthzService) DeleteTuples(ctx context.Context, tuples []authz.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tuple := range tuples {
		delete(m.tuples, tupleKey(tuple))
	}
	return nil
}

// ListRelations returns all relations for an object.
func (m *MockAuthzService) ListRelations(ctx context.Context, object authz.ObjectRef) ([]authz.RelationTuple, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []authz.RelationTuple
	for _, tuple := range m.tuples {
		if tuple.Object.Namespace == object.Namespace && tuple.Object.ID == object.ID {
			result = append(result, tuple)
		}
	}
	return result, nil
}

// ListSubjectRelations returns all objects a subject has relations to.
func (m *MockAuthzService) ListSubjectRelations(ctx context.Context, subject authz.SubjectRef, namespace string) ([]authz.RelationTuple, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []authz.RelationTuple
	for _, tuple := range m.tuples {
		if namespace != "" && tuple.Object.Namespace != namespace {
			continue
		}
		if tuple.Subject.ID == subject.ID && tuple.Subject.Namespace == subject.Namespace {
			if subject.Relation == "" || tuple.Subject.Relation == subject.Relation {
				result = append(result, tuple)
			}
		}
	}
	return result, nil
}

// Expand returns all subjects with a given relation.
func (m *MockAuthzService) Expand(ctx context.Context, object authz.ObjectRef, relation string) ([]authz.SubjectRef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []authz.SubjectRef
	for _, tuple := range m.tuples {
		if tuple.Object.Namespace == object.Namespace &&
			tuple.Object.ID == object.ID &&
			tuple.Relation == relation {
			result = append(result, tuple.Subject)
		}
	}
	return result, nil
}

// Reset clears all tuples from the mock service.
func (m *MockAuthzService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tuples = make(map[string]authz.RelationTuple)
	m.AllowAll = false
	m.CheckFunc = nil
}

// GetTuples returns all stored tuples (for testing verification).
func (m *MockAuthzService) GetTuples() []authz.RelationTuple {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]authz.RelationTuple, 0, len(m.tuples))
	for _, tuple := range m.tuples {
		result = append(result, tuple)
	}
	return result
}

// HasTuple checks if a specific tuple exists (for testing verification).
func (m *MockAuthzService) HasTuple(tuple authz.RelationTuple) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.tuples[tupleKey(tuple)]
	return exists
}

// AddRoomMember is a convenience method for testing.
func (m *MockAuthzService) AddRoomMember(roomID, profileID, role string) error {
	return m.WriteTuple(context.Background(), authz.RelationTuple{
		Object:   authz.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: authz.RoleToRelation(role),
		Subject:  authz.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	})
}

// MockAuthzMiddleware wraps the mock service with the middleware interface.
type MockAuthzMiddleware struct {
	*authzMiddlewareImpl
	MockService *MockAuthzService
}

// authzMiddlewareImpl is a copy of the middleware implementation for mocking.
type authzMiddlewareImpl struct {
	authz.AuthzMiddleware
}

// NewMockAuthzMiddleware creates a new mock middleware with an underlying mock service.
func NewMockAuthzMiddleware() *MockAuthzMiddleware {
	mockService := NewMockAuthzService()
	middleware := authz.NewAuthzMiddleware(mockService, nil)

	return &MockAuthzMiddleware{
		authzMiddlewareImpl: &authzMiddlewareImpl{
			AuthzMiddleware: middleware,
		},
		MockService: mockService,
	}
}

// ValidRoles returns the list of valid roles for testing.
func ValidRoles() []string {
	return []string{authz.RoleOwner, authz.RoleAdmin, authz.RoleMember, authz.RoleGuest}
}

// IsValidRole checks if a role is valid.
func IsValidRole(role string) bool {
	return slices.Contains(ValidRoles(), role)
}
