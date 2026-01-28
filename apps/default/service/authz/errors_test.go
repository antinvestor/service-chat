package authz_test

import (
	"errors"
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionDeniedError(t *testing.T) {
	err := authorizer.NewPermissionDeniedError(
		security.ObjectRef{Namespace: authz.NamespaceRoom, ID: "room123"},
		authz.PermissionView,
		security.SubjectRef{Namespace: authz.NamespaceProfile, ID: "user456"},
		"not a member",
	)

	// Test error message
	assert.Contains(t, err.Error(), "permission denied")
	assert.Contains(t, err.Error(), "user456")
	assert.Contains(t, err.Error(), "view")
	assert.Contains(t, err.Error(), "room:room123")
	assert.Contains(t, err.Error(), "not a member")

	// Test Is() method
	require.ErrorIs(t, err, authorizer.ErrPermissionDenied)
	require.NotErrorIs(t, err, authorizer.ErrInvalidObject)

	// Test Unwrap() method
	assert.Equal(t, authorizer.ErrPermissionDenied, err.Unwrap())

	// Test error fields
	assert.Equal(t, authz.NamespaceRoom, err.Object.Namespace)
	assert.Equal(t, "room123", err.Object.ID)
	assert.Equal(t, authz.PermissionView, err.Permission)
	assert.Equal(t, "user456", err.Subject.ID)
	assert.Equal(t, "not a member", err.Reason)
}

func TestAuthzServiceError(t *testing.T) {
	cause := errors.New("connection refused")
	err := authorizer.NewAuthzServiceError("check", cause)

	// Test error message
	assert.Contains(t, err.Error(), "authz service error")
	assert.Contains(t, err.Error(), "check")
	assert.Contains(t, err.Error(), "connection refused")

	// Test Is() method
	require.ErrorIs(t, err, authorizer.ErrAuthzServiceDown)
	require.NotErrorIs(t, err, authorizer.ErrPermissionDenied)

	// Test Unwrap() method
	assert.Equal(t, cause, err.Unwrap())

	// Test error fields
	assert.Equal(t, "check", err.Operation)
	assert.Equal(t, cause, err.Cause)
}

func TestStandardErrors(t *testing.T) {
	// Test that all standard errors are distinct
	errs := []error{
		authorizer.ErrPermissionDenied,
		authorizer.ErrInvalidObject,
		authorizer.ErrInvalidSubject,
		authorizer.ErrTupleNotFound,
		authorizer.ErrTupleAlreadyExists,
		authorizer.ErrAuthzServiceDown,
		authorizer.ErrInvalidPermission,
		authorizer.ErrInvalidRole,
	}

	for i, err1 := range errs {
		for j, err2 := range errs {
			if i == j {
				require.ErrorIs(t, err1, err2)
			} else {
				require.NotErrorIs(t, err1, err2, "error %d and %d should not match", i, j)
			}
		}
	}

	// Test error messages
	assert.Equal(t, "permission denied", authorizer.ErrPermissionDenied.Error())
	assert.Equal(t, "invalid object reference", authorizer.ErrInvalidObject.Error())
	assert.Equal(t, "invalid subject reference", authorizer.ErrInvalidSubject.Error())
	assert.Equal(t, "relationship tuple not found", authorizer.ErrTupleNotFound.Error())
	assert.Equal(t, "relationship tuple already exists", authorizer.ErrTupleAlreadyExists.Error())
	assert.Equal(t, "authorization service unavailable", authorizer.ErrAuthzServiceDown.Error())
	assert.Equal(t, "invalid permission", authorizer.ErrInvalidPermission.Error())
	assert.Equal(t, "invalid role", authorizer.ErrInvalidRole.Error())
}
