package config

import (
	"fmt"

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
}

// ValidateSharding checks that shard configuration is internally consistent.
// ShardCount must be positive and must match the number of gateway queue URIs.
func (c *ChatConfig) ValidateSharding() error {
	if c.ShardCount <= 0 {
		return fmt.Errorf("SHARD_COUNT must be > 0, got %d", c.ShardCount)
	}

	if len(c.QueueGatewayEventDeliveryURI) != c.ShardCount {
		return fmt.Errorf(
			"SHARD_COUNT (%d) must match number of QUEUE_GATEWAY_EVENT_DELIVERY_URI entries (%d)",
			c.ShardCount, len(c.QueueGatewayEventDeliveryURI))
	}

	return nil
}
