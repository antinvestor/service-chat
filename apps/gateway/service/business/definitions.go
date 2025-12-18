package business

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/internal"
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
	return internal.MetadataKey(cm.ProfileID, cm.DeviceID)
}

type Connection interface {
	Lock()
	Unlock()
	Metadata() *Metadata
	Dispatch(*chatv1.ConnectResponse) bool // Returns false if channel is full
	ConsumeDispatch(ctx context.Context) *chatv1.ConnectResponse
	Stream() DeviceStream
}

// DeviceStream abstracts the bidirectional stream for edge devices.
type DeviceStream interface {
	Receive() (*chatv1.ConnectRequest, error)
	Send(response *chatv1.ConnectResponse) error
}

type ConnectionManager interface {
	HandleConnection(
		ctx context.Context,
		profileID string,
		deviceID string,
		stream DeviceStream,
	) error
	GetConnection(
		ctx context.Context,
		profileID string,
		deviceID string,
	) (Connection, bool)
}
