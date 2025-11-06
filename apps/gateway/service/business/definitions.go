package business

import (
	"context"
	"fmt"
	"sync"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/pitabwire/frame/queue"
)

// Metadata represents the cached connection metadata.
type Metadata struct {
	ProfileID     string `json:"profile_id"`
	DeviceID      string `json:"device_id"`
	LastActive    int64  `json:"last_active"`    // Unix timestamp
	LastHeartbeat int64  `json:"last_heartbeat"` // Unix timestamp
	Connected     int64  `json:"connected"`      // Unix timestamp
	GatewayID     string `json:"gateway_id"`     // Which gateway instance owns this connection
}

func (cm *Metadata) Key() string {
	return fmt.Sprintf("%s:%s", cm.ProfileID, cm.DeviceID)
}

// Connection represents an active edge device connection.
type Connection struct {
	metadata   *Metadata
	subscriber queue.Subscriber
	stream     DeviceStream
	mu         sync.RWMutex
}

// DeviceStream abstracts the bidirectional stream for edge devices.
type DeviceStream interface {
	Receive() (*chatv1.ConnectRequest, error)
	Send(*chatv1.ServerEvent) error
}

type ConnectionManager interface {
	HandleConnection(
		ctx context.Context,
		profileID string,
		deviceID string,
		stream DeviceStream,
	) error
}
