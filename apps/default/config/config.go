package config

import (
	"errors"
	"fmt"
	"strings"

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

	// Dead-letter queue for deliveries that exceed max retries
	QueueDeadLetterName string `envDefault:"dead.letter.queue"       env:"QUEUE_DEAD_LETTER_NAME"`
	QueueDeadLetterURI  string `envDefault:"mem://dead.letter.queue" env:"QUEUE_DEAD_LETTER_URI"`
	MaxDeliveryRetries  int    `envDefault:"5"                       env:"MAX_DELIVERY_RETRIES"`
}

// Validate checks that the configuration is valid.
// Returns an error if any validation fails.
func (c *ChatConfig) Validate() error {
	var errs []error

	// Validate ShardCount
	if c.ShardCount <= 0 {
		errs = append(errs, errors.New("ShardCount must be > 0"))
	}

	// Validate ShardCount matches gateway queue URIs
	if len(c.QueueGatewayEventDeliveryURI) != c.ShardCount {
		errs = append(errs, fmt.Errorf("QueueGatewayEventDeliveryURI count (%d) must match ShardCount (%d)",
			len(c.QueueGatewayEventDeliveryURI), c.ShardCount))
	}

	// Validate queue URIs
	if err := validateQueueURI(c.QueueDeviceEventDeliveryURI, "QueueDeviceEventDeliveryURI"); err != nil {
		errs = append(errs, err)
	}
	if err := validateQueueURI(c.QueueOfflineEventDeliveryURI, "QueueOfflineEventDeliveryURI"); err != nil {
		errs = append(errs, err)
	}
	for i, uri := range c.QueueGatewayEventDeliveryURI {
		if err := validateQueueURI(uri, fmt.Sprintf("QueueGatewayEventDeliveryURI[%d]", i)); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
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
