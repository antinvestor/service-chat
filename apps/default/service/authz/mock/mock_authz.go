package mock

import (
	"context"
	"sync"

	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/pitabwire/frame/security"
)

// rolePermissions maps relation names to their allowed permissions.
var rolePermissions = map[string][]string{ //nolint:gochecknoglobals // lookup table for test mock
	authz.RelationOwner: {
		authz.PermissionView, authz.PermissionSendMessage, authz.PermissionUpdate,
		authz.PermissionDelete, authz.PermissionManageMembers, authz.PermissionManageRoles,
		authz.PermissionDeleteAnyMessage, authz.PermissionEdit, authz.PermissionReact,
	},
	authz.RelationAdmin: {
		authz.PermissionView, authz.PermissionSendMessage, authz.PermissionUpdate,
		authz.PermissionManageMembers, authz.PermissionDeleteAnyMessage,
		authz.PermissionEdit, authz.PermissionReact,
	},
	authz.RelationMember: {
		authz.PermissionView, authz.PermissionSendMessage,
		authz.PermissionEdit, authz.PermissionReact,
	},
	authz.RelationViewer: {
		authz.PermissionView,
	},
}

// MockAuthzService is an in-memory implementation of security.Authorizer for tests.
type MockAuthzService struct {
	mu     sync.RWMutex
	tuples []security.RelationTuple

	// CheckFunc allows overriding the default Check behavior.
	CheckFunc func(ctx context.Context, req security.CheckRequest) (security.CheckResult, error)
}

// NewMockAuthzService creates a new mock authorizer.
func NewMockAuthzService() *MockAuthzService {
	return &MockAuthzService{}
}

// AddRoomMember is a convenience method to add a member tuple.
func (m *MockAuthzService) AddRoomMember(roomID, profileID, role string) error {
	relation := authz.RoleToRelation(role)
	return m.WriteTuple(context.Background(), security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: relation,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	})
}

// HasTuple checks if a specific tuple exists.
func (m *MockAuthzService) HasTuple(tuple security.RelationTuple) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, t := range m.tuples {
		if t.Object == tuple.Object && t.Relation == tuple.Relation &&
			t.Subject.Namespace == tuple.Subject.Namespace && t.Subject.ID == tuple.Subject.ID {
			return true
		}
	}
	return false
}

// GetTuples returns all stored tuples.
func (m *MockAuthzService) GetTuples() []security.RelationTuple {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]security.RelationTuple, len(m.tuples))
	copy(result, m.tuples)
	return result
}

// Reset clears all tuples and overrides.
func (m *MockAuthzService) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tuples = nil
	m.CheckFunc = nil
}

// Check implements security.Authorizer.
func (m *MockAuthzService) Check(ctx context.Context, req security.CheckRequest) (security.CheckResult, error) {
	if m.CheckFunc != nil {
		return m.CheckFunc(ctx, req)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, t := range m.tuples {
		if t.Object == req.Object && t.Subject.Namespace == req.Subject.Namespace && t.Subject.ID == req.Subject.ID {
			perms, ok := rolePermissions[t.Relation]
			if !ok {
				continue
			}
			for _, p := range perms {
				if p == req.Permission {
					return security.CheckResult{Allowed: true}, nil
				}
			}
		}
	}
	return security.CheckResult{Allowed: false, Reason: "no matching tuple"}, nil
}

// BatchCheck implements security.Authorizer.
func (m *MockAuthzService) BatchCheck(ctx context.Context, requests []security.CheckRequest) ([]security.CheckResult, error) {
	results := make([]security.CheckResult, len(requests))
	for i, req := range requests {
		result, err := m.Check(ctx, req)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}
	return results, nil
}

// WriteTuple implements security.Authorizer.
func (m *MockAuthzService) WriteTuple(_ context.Context, tuple security.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tuples = append(m.tuples, tuple)
	return nil
}

// WriteTuples implements security.Authorizer.
func (m *MockAuthzService) WriteTuples(_ context.Context, tuples []security.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tuples = append(m.tuples, tuples...)
	return nil
}

// DeleteTuple implements security.Authorizer.
func (m *MockAuthzService) DeleteTuple(_ context.Context, tuple security.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.tuples {
		if t.Object == tuple.Object && t.Relation == tuple.Relation &&
			t.Subject.Namespace == tuple.Subject.Namespace && t.Subject.ID == tuple.Subject.ID {
			m.tuples = append(m.tuples[:i], m.tuples[i+1:]...)
			return nil
		}
	}
	return nil
}

// DeleteTuples implements security.Authorizer.
func (m *MockAuthzService) DeleteTuples(_ context.Context, tuples []security.RelationTuple) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tuple := range tuples {
		for i, t := range m.tuples {
			if t.Object == tuple.Object && t.Relation == tuple.Relation &&
				t.Subject.Namespace == tuple.Subject.Namespace && t.Subject.ID == tuple.Subject.ID {
				m.tuples = append(m.tuples[:i], m.tuples[i+1:]...)
				break
			}
		}
	}
	return nil
}

// ListRelations implements security.Authorizer.
func (m *MockAuthzService) ListRelations(_ context.Context, object security.ObjectRef) ([]security.RelationTuple, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []security.RelationTuple
	for _, t := range m.tuples {
		if t.Object == object {
			result = append(result, t)
		}
	}
	return result, nil
}

// ListSubjectRelations implements security.Authorizer.
func (m *MockAuthzService) ListSubjectRelations(_ context.Context, subject security.SubjectRef, namespace string) ([]security.RelationTuple, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []security.RelationTuple
	for _, t := range m.tuples {
		if t.Subject.Namespace == subject.Namespace && t.Subject.ID == subject.ID && t.Object.Namespace == namespace {
			result = append(result, t)
		}
	}
	return result, nil
}

// Expand implements security.Authorizer.
func (m *MockAuthzService) Expand(_ context.Context, object security.ObjectRef, relation string) ([]security.SubjectRef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []security.SubjectRef
	for _, t := range m.tuples {
		if t.Object == object && t.Relation == relation {
			result = append(result, t.Subject)
		}
	}
	return result, nil
}
