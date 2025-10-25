package business

import (
	"context"
	"fmt"
	"sync"
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ConnectionMetadata represents the cached connection metadata.
type ConnectionMetadata struct {
	ProfileID     string `json:"profile_id"`
	DeviceID      string `json:"device_id"`
	LastActive    int64  `json:"last_active"`    // Unix timestamp
	LastHeartbeat int64  `json:"last_heartbeat"` // Unix timestamp
	Connected     int64  `json:"connected"`      // Unix timestamp
	GatewayID     string `json:"gateway_id"`     // Which gateway instance owns this connection
}

func (cm *ConnectionMetadata) Key() string {
	return fmt.Sprintf("%s:%s", cm.ProfileID, cm.DeviceID)

}

// DeviceConnection represents an active edge device connection.
type DeviceConnection struct {
	Metadata *ConnectionMetadata
	Stream   DeviceStream
	mu       sync.RWMutex
}

// DeviceStream abstracts the bidirectional stream for edge devices.
type DeviceStream interface {
	Receive() (*chatv1.ConnectRequest, error)
	Send(*chatv1.ServerEvent) error
}

// ConnectionManager manages all active device connections.
type ConnectionManager struct {
	connCache cache.Cache[string, ConnectionMetadata]

	// Local stream tracking (not cacheable)
	localStreams map[string]*DeviceConnection // key: profileID:deviceID
	streamMu     sync.RWMutex

	// Gateway instance ID
	gatewayID string

	// Cache key prefixes
	connPrefix string // "gateway:conn:"
	roomPrefix string // "gateway:room:"

	// Configuration
	maxConnectionsPerDevice int
	connectionTimeoutSec    int
	heartbeatIntervalSec    int
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(
	rawCache cache.RawCache,
	maxConnectionsPerDevice int,
	connectionTimeoutSec int,
	heartbeatIntervalSec int,
) *ConnectionManager {
	// Generate a unique gateway instance ID
	gatewayID := fmt.Sprintf("gateway-%d", time.Now().UnixNano())

	cm := &ConnectionManager{

		connCache: cache.NewGenericCache[string, ConnectionMetadata](rawCache, func(s string) string {
			return s
		}),

		localStreams:            make(map[string]*DeviceConnection),
		gatewayID:               gatewayID,
		connPrefix:              "gateway:conn:",
		roomPrefix:              "gateway:room:",
		maxConnectionsPerDevice: maxConnectionsPerDevice,
		connectionTimeoutSec:    connectionTimeoutSec,
		heartbeatIntervalSec:    heartbeatIntervalSec,
	}

	// Start background cleanup routine
	go cm.cleanupStaleConnections()

	return cm
}

// HandleConnection manages a device connection lifecycle.
func (cm *ConnectionManager) HandleConnection(
	ctx context.Context,
	profileID string,
	deviceID string,
	stream DeviceStream,
) error {
	if profileID == "" || deviceID == "" {
		return fmt.Errorf("profileID and deviceID are required")
	}

	// Create new connection
	now := time.Now()

	// Store connection metadata in cache
	metadata := &ConnectionMetadata{
		ProfileID:     profileID,
		DeviceID:      deviceID,
		LastActive:    now.Unix(),
		LastHeartbeat: now.Unix(),
		Connected:     now.Unix(),
		GatewayID:     cm.gatewayID,
	}

	// Check if connection already exists in cache
	existingMeta, err := cm.getMetadata(ctx, metadata.Key())
	if err == nil && existingMeta != nil {
		// Connection exists - remove it first
		cm.disconnect(ctx, metadata.Key())
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Info("Replacing existing connection")
	}

	conn := &DeviceConnection{
		Metadata: metadata,
		Stream:   stream,
	}

	err = cm.connect(ctx, metadata)
	if err != nil {
		return fmt.Errorf("failed to save connection metadata: %w", err)
	}

	// Register local stream
	cm.streamMu.Lock()
	cm.localStreams[metadata.Key()] = conn
	cm.streamMu.Unlock()

	util.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"device_id":  deviceID,
		"gateway_id": cm.gatewayID,
	}).Info("Device connected to gateway")

	payload := data.JSONMap{
		"status": "connected",
	}

	// Cleanup on disconnect
	defer func() {
		cm.disconnect(ctx, metadata.Key())
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Info("Device disconnected from gateway")
	}()

	// Send connection acknowledgment
	ack := &chatv1.ServerEvent{
		Payload: &chatv1.ServerEvent_Message{
			Message: &chatv1.RoomEvent{
				Id:       util.IDString(),
				Type:     chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
				Payload:  payload.ToProtoStruct(),
				SentAt:   timestamppb.Now(),
				Edited:   false,
				Redacted: false,
			},
		},
	}
	err = stream.Send(ack)
	if err != nil {
		return fmt.Errorf("failed to ack connection: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:

			req, err := stream.Receive()
			if err != nil {
				return err
			}

			// Update last active time
			conn.mu.Lock()
			conn.Metadata.LastActive = time.Now().Unix()
			conn.mu.Unlock()

			// Update in cache
			cm.updateLastActive(ctx, metadata.Key())

			// Handle device commands
			if err := cm.handleDeviceCommand(ctx, conn, req); err != nil {
				util.Log(ctx).WithError(err).Error("Error handling device command")
			}

		}
	}
}

// handleDeviceCommand processes commands from edge devices.
func (cm *ConnectionManager) handleDeviceCommand(
	ctx context.Context,
	conn *DeviceConnection,
	req *chatv1.ConnectRequest,
) error {
	switch cmd := req.GetPayload().(type) {
	case *chatv1.ConnectRequest_Command:
		return cm.processCommand(ctx, conn, cmd.Command)
	case *chatv1.ConnectRequest_Ack:
		// TODO: Device acknowledged a message
		// Queue acknowledgement for read status update
		// cmd.Ack

		cm.updateLastActive(ctx, conn.Metadata.Key())
		return nil
	default:
		return nil
	}
}

// processCommand handles specific device commands.
func (cm *ConnectionManager) processCommand(
	ctx context.Context,
	conn *DeviceConnection,
	cmd *chatv1.ClientCommand,
) error {
	// Log the command
	util.Log(ctx).WithFields(map[string]any{
		"metadata": conn.Metadata,
		"command":  cmd,
	}).Debug("Received device command")

	// Commands are typically forwarded to the chat service or handled locally
	// For now, we just acknowledge them
	return nil
}

// GetConnectionCount returns the total number of active connections.
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.streamMu.RLock()
	defer cm.streamMu.RUnlock()
	return len(cm.localStreams)
}

// Helper methods

// disconnect removes a connection from cache and local storage.
func (cm *ConnectionManager) disconnect(ctx context.Context, connKey string) {
	// Get connection metadata to find room memberships
	metadata, err := cm.getMetadata(ctx, connKey)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to get connection metadata")
		return
	}

	// Remove from local streams
	cm.streamMu.Lock()
	delete(cm.localStreams, metadata.Key())
	cm.streamMu.Unlock()

	// Remove connection metadata from cache
	_ = cm.connCache.Delete(ctx, metadata.Key())
}

func (cm *ConnectionManager) cleanupStaleConnections() {
	ticker := time.NewTicker(time.Duration(cm.heartbeatIntervalSec) * time.Second)
	defer ticker.Stop()
	ctx := context.Background()

	for range ticker.C {
		cm.streamMu.RLock()
		var staleConnections []string
		timeout := time.Duration(cm.connectionTimeoutSec) * time.Second

		for connKey, conn := range cm.localStreams {
			conn.mu.RLock()
			timeSinceActive := time.Since(time.Unix(conn.Metadata.LastActive, 0))
			conn.mu.RUnlock()

			if timeSinceActive > timeout {
				staleConnections = append(staleConnections, connKey)
			}
		}
		cm.streamMu.RUnlock()

		// Remove stale connections
		for _, connKey := range staleConnections {
			cm.disconnect(ctx, connKey)
		}
	}
}

// Cache helper methods

// connect saves connection metadata to cache.
func (cm *ConnectionManager) connect(ctx context.Context, metadata *ConnectionMetadata) error {
	cacheKey := cm.connPrefix + metadata.Key()

	ttl := time.Duration(cm.connectionTimeoutSec*2) * time.Second
	return cm.connCache.Set(ctx, cacheKey, *metadata, ttl)
}

// getMetadata retrieves connection metadata from cache.
func (cm *ConnectionManager) getMetadata(ctx context.Context, connKey string) (*ConnectionMetadata, error) {
	cacheKey := cm.connPrefix + connKey
	metadata, ok, err := cm.connCache.Get(ctx, cacheKey)
	if err != nil {
		return nil, err
	}

	if ok {
		return &metadata, nil
	}

	return nil, nil
}

// updateLastActive updates the last active timestamp in cache.
func (cm *ConnectionManager) updateLastActive(ctx context.Context, connKey string) {
	metadata, err := cm.getMetadata(ctx, connKey)
	if err == nil && metadata != nil {
		metadata.LastActive = time.Now().Unix()
		metadata.LastHeartbeat = time.Now().Unix()
		_ = cm.connect(ctx, metadata)
	}
}
