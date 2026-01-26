package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartialBatchError_Error(t *testing.T) {
	pbe := &PartialBatchError{
		Succeeded: 3,
		Failed:    2,
		Errors: []ItemError{
			{Index: 1, ItemID: "user1", Message: "validation failed"},
			{Index: 3, ItemID: "user3", Message: "profile not found"},
		},
	}

	msg := pbe.Error()
	assert.Contains(t, msg, "partial batch failure: 3 succeeded, 2 failed")
	assert.Contains(t, msg, "item 1 (user1): validation failed")
	assert.Contains(t, msg, "item 3 (user3): profile not found")
}

func TestPartialBatchError_ErrorNoItems(t *testing.T) {
	pbe := &PartialBatchError{
		Succeeded: 5,
		Failed:    0,
		Errors:    nil,
	}

	msg := pbe.Error()
	assert.Contains(t, msg, "5 succeeded, 0 failed")
}

func TestPartialBatchError_ErrorAllFailed(t *testing.T) {
	pbe := &PartialBatchError{
		Succeeded: 0,
		Failed:    3,
		Errors: []ItemError{
			{Index: 0, ItemID: "a", Message: "err1"},
			{Index: 1, ItemID: "b", Message: "err2"},
			{Index: 2, ItemID: "c", Message: "err3"},
		},
	}

	msg := pbe.Error()
	assert.Contains(t, msg, "0 succeeded, 3 failed")
	assert.Contains(t, msg, "item 0 (a): err1")
	assert.Contains(t, msg, "item 2 (c): err3")
}

func TestIsPartialBatchError_Match(t *testing.T) {
	pbe := &PartialBatchError{
		Succeeded: 1,
		Failed:    1,
		Errors:    []ItemError{{Index: 0, ItemID: "x", Message: "fail"}},
	}

	result, ok := IsPartialBatchError(pbe)
	require.True(t, ok)
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, 1, result.Failed)
}

func TestIsPartialBatchError_Wrapped(t *testing.T) {
	pbe := &PartialBatchError{
		Succeeded: 2,
		Failed:    1,
		Errors:    []ItemError{{Index: 0, ItemID: "x", Message: "fail"}},
	}

	wrapped := fmt.Errorf("outer error: %w", pbe)
	result, ok := IsPartialBatchError(wrapped)
	require.True(t, ok)
	assert.Equal(t, 2, result.Succeeded)
}

func TestIsPartialBatchError_NoMatch(t *testing.T) {
	err := errors.New("not a batch error")
	result, ok := IsPartialBatchError(err)
	assert.False(t, ok)
	assert.Nil(t, result)
}

func TestIsPartialBatchError_Nil(t *testing.T) {
	result, ok := IsPartialBatchError(nil)
	assert.False(t, ok)
	assert.Nil(t, result)
}

func TestItemError_Fields(t *testing.T) {
	ie := ItemError{
		Index:   5,
		ItemID:  "item-123",
		Message: "something went wrong",
	}

	assert.Equal(t, 5, ie.Index)
	assert.Equal(t, "item-123", ie.ItemID)
	assert.Equal(t, "something went wrong", ie.Message)
}
