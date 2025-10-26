package business

import (
	"context"
	"fmt"
	"sync"
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// cm manages all active device connections.
type connectionManager struct {
	qManager  queue.Manager
	connCache cache.Cache[string, Metadata]

	// Local stream tracking (not cacheable)
	streamMu sync.RWMutex

	// Gateway instance ID
	gatewayID string

	// Configuration
	maxConnectionsPerDevice int
	connectionTimeoutSec    int
	heartbeatIntervalSec    int
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(
	qManager queue.Manager,
	rawCache cache.RawCache,
	maxConnectionsPerDevice int,
	connectionTimeoutSec int,
	heartbeatIntervalSec int,
) ConnectionManager {
	// Generate a unique gateway instance ID
	gatewayID := fmt.Sprintf("gateway-%d", time.Now().UnixNano())

	cm := &connectionManager{
		qManager: qManager,
		connCache: cache.NewGenericCache[string, Metadata](rawCache, func(s string) string {
			return s
		}),

		gatewayID: gatewayID,

		maxConnectionsPerDevice: maxConnectionsPerDevice,
		connectionTimeoutSec:    connectionTimeoutSec,
		heartbeatIntervalSec:    heartbeatIntervalSec,
	}

	return cm
}

// HandleConnection manages a device connection lifecycle.
func (cm *connectionManager) HandleConnection(
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
	metadata := &Metadata{
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

	conn := &Connection{
		metadata: metadata,
		stream:   stream,
	}

	err = cm.connect(ctx, metadata)
	if err != nil {
		return fmt.Errorf("failed to save connection metadata: %w", err)
	}

	util.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"device_id":  deviceID,
		"gateway_id": cm.gatewayID,
	}).Debug("Device connected to gateway")

	// Cleanup on disconnect
	defer func() {
		cm.disconnect(ctx, metadata.Key())
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Debug("Device disconnected from gateway")
	}()

	wg := new(sync.WaitGroup)
	errChan := make(chan error, 2)

	// Launch functions as goroutines
	wg.Go(func() {
		for {
			select {
			case <-errChan:
				return
			case <-ctx.Done():
				return

			default:

				req, receiveErr := stream.Receive()
				if receiveErr != nil {
					errChan <- receiveErr
				}

				inboundErr := cm.handleInboundRequests(ctx, conn, req)
				if inboundErr != nil {
					util.Log(ctx).WithError(inboundErr).WithField("req", req).Error("failure handling inbound request")
					errChan <- receiveErr
				}
			}
		}
	})

	wg.Go(func() {

		payload := data.JSONMap{
			"status": "connected",
		}

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
			errChan <- fmt.Errorf("connection ack failed: %w", err)
			return
		}

		for {
			select {
			case <-errChan:
				return
			case <-ctx.Done():
				return

			default:

				req, qErr := conn.subscriber.Receive(ctx)
				if qErr != nil {
					errChan <- qErr
					return
				}

				finalMsg, qErr := toUserDelivery(req)
				if qErr != nil {
					errChan <- qErr
					return
				}

				outErr := cm.handleOutboundRequests(ctx, conn, finalMsg)
				if outErr != nil {
					util.Log(ctx).WithError(outErr).WithField("req", req).Error("failure handling inbound request")
					errChan <- qErr
					return
				}

				req.Ack()
			}
		}
	})

	wg.Wait()

	return nil
}

// Helper methods

// disconnect removes a connection from cache and local storage.
func (cm *connectionManager) disconnect(ctx context.Context, connKey string) {
	// Get connection metadata to find room memberships
	metadata, err := cm.getMetadata(ctx, connKey)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to get connection metadata")
		return
	}

	// Remove connection metadata from cache
	_ = cm.connCache.Delete(ctx, metadata.Key())
}

// Cache helper methods

// connect saves connection metadata to cache.
func (cm *connectionManager) connect(ctx context.Context, metadata *Metadata) error {

	ttl := time.Duration(cm.connectionTimeoutSec*2) * time.Second
	return cm.connCache.Set(ctx, metadata.Key(), *metadata, ttl)
}

// getMetadata retrieves connection metadata from cache.
func (cm *connectionManager) getMetadata(ctx context.Context, connKey string) (*Metadata, error) {

	metadata, ok, err := cm.connCache.Get(ctx, connKey)
	if err != nil {
		return nil, err
	}

	if ok {
		return &metadata, nil
	}

	return nil, nil
}
