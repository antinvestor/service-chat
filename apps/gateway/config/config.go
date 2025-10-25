package config

import "github.com/pitabwire/frame/config"

type GatewayConfig struct {
	config.ConfigurationDefault

	// Chat service configuration - the gateway connects to the default chat service
	ChatServiceURI string `envDefault:"127.0.0.1:7010" env:"CHAT_SERVICE_URI"`

	// Connection management
	MaxConnectionsPerDevice int `envDefault:"1" env:"MAX_CONNECTIONS_PER_DEVICE"`
	ConnectionTimeoutSec    int `envDefault:"300" env:"CONNECTION_TIMEOUT_SEC"`
	HeartbeatIntervalSec    int `envDefault:"30" env:"HEARTBEAT_INTERVAL_SEC"`

	// Rate limiting
	MaxEventsPerSecond int `envDefault:"100" env:"MAX_EVENTS_PER_SECOND"`

	// Cache configuration (Redis or similar)
	// Connection metadata is stored in cache to enable horizontal scaling
	// and allow multiple gateway instances to coordinate
	CacheName string `envDefault:"defaultCache" env:"CACHE_NAME"`
	CacheURI  string `envDefault:"redis://localhost:6379" env:"CACHE_URI"`
}
