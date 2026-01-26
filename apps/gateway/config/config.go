package config

import (
	"fmt"

	"github.com/pitabwire/frame/config"
)

type GatewayConfig struct {
	config.ConfigurationDefault

	// Chat service configuration - the gateway connects to the default chat service
	ChatServiceURI string `envDefault:"127.0.0.1:7010" env:"CHAT_SERVICE_URI"`

	// Device service configuration - for delivery status tracking
	DeviceServiceURI string `envDefault:"device.api.antinvestor.com:443" env:"DEVICE_SERVICE_URI"`

	// Connection management
	MaxConnectionsPerDevice int `envDefault:"1"   env:"MAX_CONNECTIONS_PER_DEVICE"`
	ConnectionTimeoutSec    int `envDefault:"300" env:"CONNECTION_TIMEOUT_SEC"`
	HeartbeatIntervalSec    int `envDefault:"30"  env:"HEARTBEAT_INTERVAL_SEC"`

	// Rate limiting
	MaxEventsPerSecond int `envDefault:"100" env:"MAX_EVENTS_PER_SECOND"`

	// Cache configuration (Redis or similar)
	// Connection metadata is stored in cache to enable horizontal scaling
	// and allow multiple gateway instances to coordinate
	CacheName            string `envDefault:"defaultCache"           env:"CACHE_NAME"`
	CacheURI             string `envDefault:"redis://localhost:6379" env:"CACHE_URI"`
	CacheCredentialsFile string `envDefault:""                       env:"CACHE_CREDENTIALS_FILE"`

	QueueOfflineEventDeliveryName string `envDefault:"offline.event.delivery"              env:"QUEUE_OFFLINE_EVENT_DELIVERY_NAME"`
	QueueOfflineEventDeliveryURI  string `envDefault:"mem://offline.device.event.delivery" env:"QUEUE_OFFLINE_EVENT_DELIVERY_URI"`

	// Queue for receiving user-targeted deliveries from default service
	QueueGatewayEventDeliveryName string `envDefault:"gateway.event.delivery"       env:"QUEUE_GATEWAY_EVENT_DELIVERY_NAME"`
	QueueGatewayEventDeliveryURI  string `envDefault:"mem://gateway.event.delivery" env:"QUEUE_GATEWAY_EVENT_DELIVERY_URI"`

	// Shard configuration - must be coordinated with the default service's ShardCount.
	// ShardID identifies this gateway instance's shard (0-indexed).
	// TotalShards must match the default service's ShardCount exactly.
	ShardID     int `envDefault:"0" env:"SHARD_ID"`
	TotalShards int `envDefault:"1" env:"TOTAL_SHARDS"`
}

// ValidateSharding checks that shard configuration is internally consistent.
// TotalShards must be positive and ShardID must be within [0, TotalShards).
// This must match the default service's ShardCount for correct message routing.
func (c *GatewayConfig) ValidateSharding() error {
	if c.TotalShards <= 0 {
		return fmt.Errorf("TOTAL_SHARDS must be > 0, got %d", c.TotalShards)
	}

	if c.ShardID < 0 {
		return fmt.Errorf("SHARD_ID must be >= 0, got %d", c.ShardID)
	}

	if c.ShardID >= c.TotalShards {
		return fmt.Errorf("SHARD_ID (%d) must be < TOTAL_SHARDS (%d)", c.ShardID, c.TotalShards)
	}

	return nil
}
