package business

import (
	"context"
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/pitabwire/util"
)

// handleInboundRequests processes commands from edge devices.
func (cm *connectionManager) handleInboundRequests(
	ctx context.Context,
	conn *Connection,
	req *chatv1.ConnectRequest,
) error {
	switch cmd := req.GetPayload().(type) {
	case *chatv1.ConnectRequest_Command:
		return cm.processCommand(ctx, conn, cmd.Command)
	case *chatv1.ConnectRequest_Ack:
		// TODO: Device acknowledged a message
		// Queue acknowledgement for read status update
		// cmd.Ack

		cm.updateLastActive(ctx, conn.metadata.Key())
		return nil
	default:
		return nil
	}
}

// processCommand handles specific device commands.
func (cm *connectionManager) processCommand(
	ctx context.Context,
	conn *Connection,
	cmd *chatv1.ClientCommand,
) error {
	// Log the command
	util.Log(ctx).WithFields(map[string]any{
		"metadata": conn.metadata,
		"command":  cmd,
	}).Debug("Received device command")

	switch cmd.GetCmd().(type) {
	case *chatv1.ClientCommand_Typing:
		// TODO: Handle typing indicator
	case *chatv1.ClientCommand_ReadMarker:
		// TODO: Handle read marker
	case *chatv1.ClientCommand_RoomEvent:
		// TODO: Handle room event
	}

	// Commands are typically forwarded to the chat service or handled locally
	// For now, we just acknowledge them
	return nil
}

// updateLastActive updates the last active timestamp in cache.
func (cm *connectionManager) updateLastActive(ctx context.Context, connKey string) {
	metadata, err := cm.getMetadata(ctx, connKey)
	if err == nil && metadata != nil {
		metadata.LastActive = time.Now().Unix()
		metadata.LastHeartbeat = time.Now().Unix()
		_ = cm.connect(ctx, metadata)
	}
}
