package config_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatConfig_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		cfg := validChatConfig()
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("ShardCount must be > 0", func(t *testing.T) {
		cfg := validChatConfig()
		cfg.ShardCount = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ShardCount must be > 0")
	})

	t.Run("ShardCount must match gateway queue URIs", func(t *testing.T) {
		cfg := validChatConfig()
		cfg.ShardCount = 3
		cfg.QueueGatewayEventDeliveryURI = []string{"mem://queue1", "mem://queue2"} // Only 2
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must match ShardCount")
	})

	t.Run("QueueDeviceEventDeliveryURI cannot be empty", func(t *testing.T) {
		cfg := validChatConfig()
		cfg.QueueDeviceEventDeliveryURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "QueueDeviceEventDeliveryURI")
	})

	t.Run("QueueDeviceEventDeliveryURI must have valid scheme", func(t *testing.T) {
		cfg := validChatConfig()
		cfg.QueueDeviceEventDeliveryURI = "invalid://queue"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid scheme")
	})

	t.Run("valid queue schemes", func(t *testing.T) {
		validSchemes := []string{
			"mem://queue",
			"redis://localhost:6379/queue",
			"amqp://localhost:5672/queue",
			"nats://localhost:4222/queue",
			"kafka://localhost:9092/queue",
		}

		for _, uri := range validSchemes {
			cfg := validChatConfig()
			cfg.QueueDeviceEventDeliveryURI = uri
			err := cfg.Validate()
			require.NoError(t, err, "should accept valid URI: %s", uri)
		}
	})

	t.Run("multiple validation errors", func(t *testing.T) {
		cfg := validChatConfig()
		cfg.ShardCount = 0
		cfg.QueueDeviceEventDeliveryURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		// Should contain multiple errors
		assert.Contains(t, err.Error(), "ShardCount")
		assert.Contains(t, err.Error(), "QueueDeviceEventDeliveryURI")
	})
}

func validChatConfig() config.ChatConfig {
	return config.ChatConfig{
		DeviceServiceURI:              "127.0.0.1:7020",
		NotificationServiceURI:        "127.0.0.1:7020",
		ProfileServiceURI:             "127.0.0.1:7003",
		PartitionServiceURI:           "127.0.0.1:7003",
		SystemAccessID:                "test-access-id",
		QueueDeviceEventDeliveryName:  "device.event.delivery",
		QueueDeviceEventDeliveryURI:   "mem://device.event.delivery",
		QueueOfflineEventDeliveryName: "offline.event.delivery",
		QueueOfflineEventDeliveryURI:  "mem://offline.device.event.delivery",
		QueueGatewayEventDeliveryName: "gateway.event.delivery.%d",
		QueueGatewayEventDeliveryURI:  []string{"mem://gateway.event.delivery.0"},
		ShardCount:                    1,
	}
}
