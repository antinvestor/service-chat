package config

import "github.com/pitabwire/frame/config"

type ChatConfig struct {
	config.ConfigurationDefault

	DeviceServiceURI       string `envDefault:"127.0.0.1:7020" env:"DEVICE_SERVICE_URI"`
	NotificationServiceURI string `envDefault:"127.0.0.1:7020" env:"NOTIFICATION_SERVICE_URI"`
	ProfileServiceURI      string `envDefault:"127.0.0.1:7003" env:"PROFILE_SERVICE_URI"`
	PartitionServiceURI    string `envDefault:"127.0.0.1:7003" env:"PARTITION_SERVICE_URI"`

	SystemAccessID string `envDefault:"c8cf0ldstmdlinc3eva0" env:"STATIC_SYSTEM_ACCESS_ID"`

	QueueUserEventDeliveryName string `envDefault:"user.event.delivery"               env:"QUEUE_USER_EVENT_DELIVERY_NAME"`
	QueueUserEventDeliveryURI  string `envDefault:"mem://user.event.delivery" env:"QUEUE_USER_EVENT_DELIVERY_URI"`
}
