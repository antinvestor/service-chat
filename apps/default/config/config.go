package config

import (
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

	// Circuit Breaker Configuration
	CBMaxFailures         int `envDefault:"5"  env:"CB_MAX_FAILURES"`
	CBResetTimeoutSec     int `envDefault:"30" env:"CB_RESET_TIMEOUT_SEC"`
	CBHalfOpenMaxRequests int `envDefault:"3"  env:"CB_HALF_OPEN_MAX_REQUESTS"`
}
