package config_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfig_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		cfg := validGatewayConfig()
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("ChatServiceURI cannot be empty", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.ChatServiceURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ChatServiceURI")
	})

	t.Run("MaxConnectionsPerDevice must be >= 1", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.MaxConnectionsPerDevice = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MaxConnectionsPerDevice")
	})

	t.Run("ConnectionTimeoutSec must be > 0", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.ConnectionTimeoutSec = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ConnectionTimeoutSec")
	})

	t.Run("HeartbeatIntervalSec must be > 0", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.HeartbeatIntervalSec = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HeartbeatIntervalSec")
	})

	t.Run("ConnectionTimeoutSec must be > HeartbeatIntervalSec", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.ConnectionTimeoutSec = 30
		cfg.HeartbeatIntervalSec = 30
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ConnectionTimeoutSec")
		assert.Contains(t, err.Error(), "HeartbeatIntervalSec")

		// Also test when timeout < heartbeat
		cfg.ConnectionTimeoutSec = 20
		cfg.HeartbeatIntervalSec = 30
		err = cfg.Validate()
		require.Error(t, err)
	})

	t.Run("MaxEventsPerSecond must be > 0", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.MaxEventsPerSecond = 0
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MaxEventsPerSecond")
	})

	t.Run("ShardID must be >= 0", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.ShardID = -1
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ShardID")
	})

	t.Run("CacheURI cannot be empty", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.CacheURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CacheURI")
	})

	t.Run("CacheURI must have valid scheme", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.CacheURI = "invalid://localhost:6379"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CacheURI")
		assert.Contains(t, err.Error(), "invalid scheme")
	})

	t.Run("valid cache URI schemes", func(t *testing.T) {
		validSchemes := []string{
			"redis://localhost:6379",
			"nats://localhost:4222",
			"mem://cache",
		}

		for _, uri := range validSchemes {
			cfg := validGatewayConfig()
			cfg.CacheURI = uri
			err := cfg.Validate()
			require.NoError(t, err, "should accept valid cache URI: %s", uri)
		}
	})

	t.Run("QueueOfflineEventDeliveryURI must be valid", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.QueueOfflineEventDeliveryURI = "invalid://queue"
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "QueueOfflineEventDeliveryURI")
	})

	t.Run("QueueGatewayEventDeliveryURI must be valid", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.QueueGatewayEventDeliveryURI = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "QueueGatewayEventDeliveryURI")
	})

	t.Run("multiple validation errors", func(t *testing.T) {
		cfg := validGatewayConfig()
		cfg.ChatServiceURI = ""
		cfg.MaxConnectionsPerDevice = 0
		cfg.MaxEventsPerSecond = 0
		err := cfg.Validate()
		require.Error(t, err)
		// Should contain multiple errors
		assert.Contains(t, err.Error(), "ChatServiceURI")
		assert.Contains(t, err.Error(), "MaxConnectionsPerDevice")
		assert.Contains(t, err.Error(), "MaxEventsPerSecond")
	})
}

func validGatewayConfig() config.GatewayConfig {
	return config.GatewayConfig{
		ChatServiceURI:                "127.0.0.1:7010",
		DeviceServiceURI:              "device.api.antinvestor.com:443",
		MaxConnectionsPerDevice:       1,
		ConnectionTimeoutSec:          300,
		HeartbeatIntervalSec:          30,
		MaxEventsPerSecond:            100,
		CacheName:                     "defaultCache",
		CacheURI:                      "redis://localhost:6379",
		CacheCredentialsFile:          "",
		QueueOfflineEventDeliveryName: "offline.event.delivery",
		QueueOfflineEventDeliveryURI:  "mem://offline.device.event.delivery",
		QueueGatewayEventDeliveryName: "gateway.event.delivery",
		QueueGatewayEventDeliveryURI:  "mem://gateway.event.delivery",
		ShardID:                       0,
	}
}
