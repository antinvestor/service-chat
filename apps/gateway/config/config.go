package config

import (
	"errors"
	"fmt"
	"strings"

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

// Validate checks that the configuration is valid.
// Returns an error if any validation fails.
func (c *GatewayConfig) Validate() error {
	var errs []error

	// Validate service URIs
	if c.ChatServiceURI == "" {
		errs = append(errs, errors.New("ChatServiceURI cannot be empty"))
	}

	// Validate connection management settings
	if c.MaxConnectionsPerDevice < 1 {
		errs = append(errs, errors.New("MaxConnectionsPerDevice must be >= 1"))
	}

	if c.ConnectionTimeoutSec <= 0 {
		errs = append(errs, errors.New("ConnectionTimeoutSec must be > 0"))
	}

	if c.HeartbeatIntervalSec <= 0 {
		errs = append(errs, errors.New("HeartbeatIntervalSec must be > 0"))
	}

	if c.ConnectionTimeoutSec <= c.HeartbeatIntervalSec {
		errs = append(errs, fmt.Errorf("ConnectionTimeoutSec (%d) must be > HeartbeatIntervalSec (%d)",
			c.ConnectionTimeoutSec, c.HeartbeatIntervalSec))
	}

	// Validate rate limiting
	if c.MaxEventsPerSecond <= 0 {
		errs = append(errs, errors.New("MaxEventsPerSecond must be > 0"))
	}

	// Validate shard configuration
	if c.ShardID < 0 {
		errs = append(errs, errors.New("ShardID must be >= 0"))
	}

	if c.TotalShards <= 0 {
		errs = append(errs, errors.New("TotalShards must be > 0"))
	}

	if c.TotalShards > 0 && c.ShardID >= c.TotalShards {
		errs = append(errs, fmt.Errorf("ShardID (%d) must be < TotalShards (%d)",
			c.ShardID, c.TotalShards))
	}

	// Validate cache configuration
	if err := validateCacheURI(c.CacheURI, "CacheURI"); err != nil {
		errs = append(errs, err)
	}

	// Validate queue URIs
	if err := validateQueueURI(c.QueueOfflineEventDeliveryURI, "QueueOfflineEventDeliveryURI"); err != nil {
		errs = append(errs, err)
	}
	if err := validateQueueURI(c.QueueGatewayEventDeliveryURI, "QueueGatewayEventDeliveryURI"); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// validateCacheURI checks that a cache URI has a valid scheme.
func validateCacheURI(uri, name string) error {
	if uri == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}

	validSchemes := []string{"redis://", "nats://", "mem://", "memory://"}
	for _, scheme := range validSchemes {
		if strings.HasPrefix(uri, scheme) {
			return nil
		}
	}

	return fmt.Errorf("%s has invalid scheme (must be one of: %s): %s", name, strings.Join(validSchemes, ", "), uri)
}

// validateQueueURI checks that a queue URI has a valid scheme.
func validateQueueURI(uri, name string) error {
	if uri == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}

	validSchemes := []string{"mem://", "redis://", "amqp://", "nats://", "kafka://"}
	for _, scheme := range validSchemes {
		if strings.HasPrefix(uri, scheme) {
			return nil
		}
	}

	return fmt.Errorf("%s has invalid scheme (must be one of: %s): %s", name, strings.Join(validSchemes, ", "), uri)
}
