package config

import (
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/authz/keto"
	"github.com/pitabwire/frame/config"
)

type ChatConfig struct {
	config.ConfigurationDefault

	DeviceServiceURI       string `envDefault:"127.0.0.1:7020" env:"DEVICE_SERVICE_URI"`
	NotificationServiceURI string `envDefault:"127.0.0.1:7020" env:"NOTIFICATION_SERVICE_URI"`
	ProfileServiceURI      string `envDefault:"127.0.0.1:7003" env:"PROFILE_SERVICE_URI"`
	PartitionServiceURI    string `envDefault:"127.0.0.1:7003" env:"PARTITION_SERVICE_URI"`

	SystemAccessID string `envDefault:"c8cf0ldstmdlinc3eva0" env:"STATIC_SYSTEM_ACCESS_ID"`

	QueueDeviceEventDeliveryName string `envDefault:"device.event.delivery"       env:"QUEUE_DEVICE_EVENT_DELIVERY_NAME"`
	QueueDeviceEventDeliveryURI  string `envDefault:"mem://device.event.delivery" env:"QUEUE_DEVICE_EVENT_DELIVERY_URI"`

	QueueOfflineEventDeliveryName string `envDefault:"offline.event.delivery"              env:"QUEUE_OFFLINE_EVENT_DELIVERY_NAME"`
	QueueOfflineEventDeliveryURI  string `envDefault:"mem://offline.device.event.delivery" env:"QUEUE_OFFLINE_EVENT_DELIVERY_URI"`

	QueueGatewayEventDeliveryName string   `envDefault:"gateway.event.delivery.%d"                                     env:"QUEUE_GATEWAY_EVENT_DELIVERY_NAME"`
	QueueGatewayEventDeliveryURI  []string `envDefault:"mem://gateway.event.delivery.0,mem://gateway.event.delivery.1" env:"QUEUE_GATEWAY_EVENT_DELIVERY_URI"`

	ShardCount int `envDefault:"1" env:"SHARD_COUNT"`

	// Keto Authorization Service Configuration
	KetoReadURL       string `envDefault:"http://localhost:4466" env:"KETO_READ_URL"`
	KetoWriteURL      string `envDefault:"http://localhost:4467" env:"KETO_WRITE_URL"`
	KetoTimeoutMs     int    `envDefault:"5000"                  env:"KETO_TIMEOUT_MS"`
	KetoRetryAttempts int    `envDefault:"3"                     env:"KETO_RETRY_ATTEMPTS"`
	KetoEnabled       bool   `envDefault:"true"                  env:"KETO_ENABLED"`

	// Audit Logging Configuration
	AuthzAuditEnabled    bool    `envDefault:"true" env:"AUTHZ_AUDIT_ENABLED"`
	AuthzAuditSampleRate float64 `envDefault:"1.0"  env:"AUTHZ_AUDIT_SAMPLE_RATE"`
}

// GetKetoConfig returns the Keto configuration derived from chat config.
func (c *ChatConfig) GetKetoConfig() keto.Config {
	return keto.Config{
		ReadURL:       c.KetoReadURL,
		WriteURL:      c.KetoWriteURL,
		Timeout:       time.Duration(c.KetoTimeoutMs) * time.Millisecond,
		RetryAttempts: c.KetoRetryAttempts,
		RetryDelay:    100 * time.Millisecond,
		Enabled:       c.KetoEnabled,
	}
}
