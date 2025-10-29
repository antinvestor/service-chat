package business

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// handleInboundRequests processes commands from edge devices.
// This is the main entry point for all client-originated messages.
func (cm *connectionManager) handleInboundRequests(
	ctx context.Context,
	conn *Connection,
	req *chatv1.ConnectRequest,
) error {
	// Update last active timestamp for all inbound requests
	defer cm.updateLastActive(ctx, conn.metadata.Key())

	switch cmd := req.GetPayload().(type) {
	case *chatv1.ConnectRequest_StateUpdate:
		return cm.processStateUpdate(ctx, conn, cmd.StateUpdate)
	case *chatv1.ConnectRequest_Ack:
		return cm.processAcknowledgement(ctx, conn, cmd.Ack)
	default:
		util.Log(ctx).WithField("payload_type", fmt.Sprintf("%T", req.GetPayload())).
			Debug("Received unknown payload type")
		return nil
	}
}

// processAcknowledgement handles device acknowledgements for delivered messages.
// This enables read receipts and delivery status tracking.
func (cm *connectionManager) processAcknowledgement(
	ctx context.Context,
	conn *Connection,
	ack *chatv1.StreamAck,
) error {
	if ack == nil {
		return nil
	}

	// If there's an error in the ack, log it for debugging
	if ack.GetError() != nil {
		util.Log(ctx).WithFields(map[string]any{
			"event_id":   ack.GetEventId(),
			"error_code": ack.GetError().GetCode(),
			"error_msg":  ack.GetError().GetMessage(),
			"device_id":  conn.metadata.DeviceID,
		}).Warn("Client reported error processing event")
		return nil
	}

	return cm.processStateUpdate(ctx, conn, &chatv1.ClientState{
		State: &chatv1.ClientState_Receipt{
			Receipt: &chatv1.ReceiptEvent{
				ProfileId: conn.metadata.ProfileID,
				RoomId:    ack.GetRoomId(),
				EventId:   []string{ack.GetEventId()},
			},
		},
	})

}

// processStateUpdate handles specific device commands.
// Commands include typing indicators, read markers, and room events (messages).
func (cm *connectionManager) processStateUpdate(
	ctx context.Context,
	conn *Connection,
	clientState *chatv1.ClientState,
) error {
	if clientState == nil {
		return nil
	}

	profileID := conn.metadata.ProfileID
	roomID := ""

	// Validate that the profile_id in ClientState matches the authenticated user
	// This prevents spoofing attacks where a client tries to impersonate another user

	// Log the command for debugging
	util.Log(ctx).WithFields(map[string]any{
		"profile_id": conn.metadata.ProfileID,
		"device_id":  conn.metadata.DeviceID,
		"state":      fmt.Sprintf("%T", clientState.GetState()),
	}).Debug("Received client state update")

	// Comprehensive validation and sanitization of all client state types
	// This prevents spoofing attacks and ensures data integrity
	switch st := clientState.GetState().(type) {
	case *chatv1.ClientState_Receipt:
		if st.Receipt != nil {

			// Validate that the profile_id in receipt matches the authenticated user
			if st.Receipt.GetProfileId() != "" && st.Receipt.GetProfileId() != profileID {
				util.Log(ctx).WithFields(map[string]any{
					"claimed_profile": st.Receipt.GetProfileId(),
					"actual_profile":  conn.metadata.ProfileID,
					"room_id":         st.Receipt.GetRoomId(),
					"event_ids":       st.Receipt.GetEventId(),
				}).Warn("Profile ID mismatch in receipt - potential spoofing attempt")
				return fmt.Errorf("receipt profile_id mismatch: claimed %s, actual %s",
					st.Receipt.GetProfileId(), conn.metadata.ProfileID)
			}
			// Always override profile_id with authenticated profile ID for security
			st.Receipt.ProfileId = profileID
			roomID = st.Receipt.GetRoomId()
		}

	case *chatv1.ClientState_ReadMarker:
		if st.ReadMarker != nil {

			// Validate that the profile_id in receipt matches the authenticated user
			if st.ReadMarker.GetProfileId() != "" && st.ReadMarker.GetProfileId() != profileID {
				util.Log(ctx).WithFields(map[string]any{
					"claimed_profile": st.ReadMarker.GetProfileId(),
					"actual_profile":  profileID,
					"room_id":         st.ReadMarker.GetRoomId(),
					"up to event_id":  st.ReadMarker.GetUpToEventId(),
				}).Warn("Profile ID mismatch in readmarker - potential spoofing attempt")
				return fmt.Errorf("receipt profile_id mismatch: claimed %s, actual %s",
					st.ReadMarker.GetProfileId(), profileID)
			}
			// Always override profile_id with authenticated profile ID for security
			st.ReadMarker.ProfileId = profileID
			roomID = st.ReadMarker.GetRoomId()
		}

	case *chatv1.ClientState_RoomEvent:
		if st.RoomEvent != nil {
			// Validate that the sender_id matches the authenticated user
			if st.RoomEvent.GetSenderId() != "" && st.RoomEvent.GetSenderId() != profileID {
				util.Log(ctx).WithFields(map[string]any{
					"claimed_sender": st.RoomEvent.GetSenderId(),
					"actual_sender":  profileID,
					"room_id":        st.RoomEvent.GetRoomId(),
					"event_type":     st.RoomEvent.GetType(),
				}).Warn("Sender ID mismatch in event - potential spoofing attempt")
				return fmt.Errorf("sender_id mismatch: claimed %s, actual %s",
					st.RoomEvent.GetSenderId(), profileID)
			}
			// Always override sender_id with authenticated profile ID for security
			st.RoomEvent.SenderId = profileID
			roomID = st.RoomEvent.GetRoomId()

			// Set timestamp if not provided
			if st.RoomEvent.GetSentAt() == nil {
				st.RoomEvent.SentAt = timestamppb.Now()
			}
		}

	case *chatv1.ClientState_Typing:
		// Handle typing indicators
		if st.Typing != nil {
			// Validate that the profile_id in typing indicator matches the authenticated user
			if st.Typing.GetProfileId() != "" && st.Typing.GetProfileId() != profileID {
				util.Log(ctx).WithFields(map[string]any{
					"claimed_profile": st.Typing.GetProfileId(),
					"actual_profile":  profileID,
					"room_id":         st.Typing.GetRoomId(),
				}).Warn("Profile ID mismatch in typing indicator - potential spoofing attempt")
				return fmt.Errorf("typing profile_id mismatch: claimed %s, actual %s",
					st.Typing.GetProfileId(), profileID)
			}
			// Always override profile_id with authenticated profile ID for security
			st.Typing.ProfileId = profileID
			roomID = st.Typing.GetRoomId()

			// Set timestamp if not provided
			if st.Typing.GetSince() == nil {
				st.Typing.Since = timestamppb.Now()
			}
		}

	case *chatv1.ClientState_Presence:
		// Handle presence updates
		if st.Presence != nil {
			// Validate that the profile_id in presence update matches the authenticated user
			if st.Presence.GetProfileId() != "" && st.Presence.GetProfileId() != profileID {
				util.Log(ctx).WithFields(map[string]any{
					"claimed_profile": st.Presence.GetProfileId(),
					"actual_profile":  profileID,
					"status":          st.Presence.GetStatus(),
				}).Warn("Profile ID mismatch in presence update - potential spoofing attempt")
				return fmt.Errorf("presence profile_id mismatch: claimed %s, actual %s",
					st.Presence.GetProfileId(), profileID)
			}
			// Always override profile_id with authenticated profile ID for security
			st.Presence.ProfileId = profileID

			// Set timestamp if not provided
			if st.Presence.GetLastActive() == nil {
				st.Presence.LastActive = timestamppb.Now()
			}
		}

	default:
		// Handle unknown state types - log for monitoring
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  conn.metadata.DeviceID,
			"state_type": fmt.Sprintf("%T", st),
		}).Warn("Received unknown client state type")
		return fmt.Errorf("unsupported client state type: %T", st)
	}

	// Forward to chat service for processing and distribution
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	updateReq := &chatv1.UpdateClientStateRequest{
		RoomId:       roomID,
		ProfileId:    profileID,
		ClientStates: []*chatv1.ClientState{clientState},
	}

	resp, err := cm.chatClient.UpdateClientState(reqCtx, connect.NewRequest(updateReq))
	if err != nil {
		util.Log(ctx).WithError(err).
			WithFields(map[string]any{
				"state": clientState,
			}).Error("Failed to update a client's state")
		return fmt.Errorf("failed to update client state: %w", err)
	}

	if resp.Msg.GetError() != nil {
		errMsg := resp.Msg.GetError()
		util.Log(ctx).
			WithFields(map[string]any{
				"state":    clientState,
				"err_code": errMsg.GetCode(),
				"err_msg":  errMsg.GetCode(),
				"extra":    errMsg.GetMeta(),
			}).Error("Failed to update a client's state")
		return fmt.Errorf("failed to update client state: %v", errMsg)
	}

	return nil

}

// updateLastActive updates the last active timestamp in cache.
// This is called for every inbound request to track device activity.
func (cm *connectionManager) updateLastActive(ctx context.Context, connKey string) {
	// Get connection from pool and update its last active time
	if conn, exists := cm.connPool.get(connKey); exists {
		conn.mu.Lock()
		now := time.Now().Unix()
		// Update the connection's metadata (thread-safe with mutex)
		conn.metadata.LastActive = now
		conn.metadata.LastHeartbeat = now
		conn.mu.Unlock()
		
		util.Log(ctx).WithField("conn_key", connKey).
			Debug("Updated last active timestamp")
	}
}
