package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetadataKey(t *testing.T) {
	tests := []struct {
		profileID string
		deviceID  string
		expected  string
	}{
		{"user123", "device456", "user123:device456"},
		{"", "", ":"},
		{"profile", "", "profile:"},
		{"", "device", ":device"},
		{"a", "b", "a:b"},
	}

	for _, tc := range tests {
		result := MetadataKey(tc.profileID, tc.deviceID)
		assert.Equal(t, tc.expected, result,
			"MetadataKey(%q, %q)", tc.profileID, tc.deviceID)
	}
}

func TestMetadataKey_Deterministic(t *testing.T) {
	result1 := MetadataKey("user", "device")
	result2 := MetadataKey("user", "device")
	assert.Equal(t, result1, result2)
}

func TestMetadataKey_DifferentInputsDifferentOutputs(t *testing.T) {
	// Verify distinct inputs produce distinct keys
	key1 := MetadataKey("user1", "device1")
	key2 := MetadataKey("user2", "device1")
	key3 := MetadataKey("user1", "device2")

	assert.NotEqual(t, key1, key2)
	assert.NotEqual(t, key1, key3)
	assert.NotEqual(t, key2, key3)
}
