package business

import (
	"context"
	"errors"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
// clientStateTimeout defines timeout for client state operations
// clientStateTimeout = 5 * time.Second // Currently unused
)

// ErrRateLimited is returned when a connection exceeds its rate limit.
var ErrRateLimited = errors.New("rate limit exceeded")

// handleInboundRequests processes commands from edge devices.
// This is the main entry point for all client-originated messages.
func (cm *connectionManager) handleInboundRequests(
	ctx context.Context,
	conn Connection,
	req *chatv1.StreamRequest,
) error {
	// Check rate limit before processing
	if !conn.AllowInbound() {
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": conn.Metadata().ProfileID,
			"device_id":  conn.Metadata().DeviceID,
		}).Warn("Request rate limited")
		return ErrRateLimited
	}

	switch cmd := req.GetPayload().(type) {
	case *chatv1.StreamRequest_SignalUpdate:
		return cm.processClientSignal(ctx, conn, cmd.SignalUpdate)
	case *chatv1.StreamRequest_Command:
		return cm.processClientCommand(ctx, conn, cmd.Command)
	case *chatv1.StreamRequest_Hello:
		// Hello is processed during connection establishment
		util.Log(ctx).Debug("Received Hello message after connection established")
		return nil
	default:
		util.Log(ctx).WithField("payload_type", fmt.Sprintf("%T", req.GetPayload())).
			Debug("Received unknown payload type")
		return nil
	}
}

// processClientSignal handles ephemeral signals from clients (acks, typing, receipts, presence).
func (cm *connectionManager) processClientSignal(
	ctx context.Context,
	conn Connection,
	signal *chatv1.ClientSignal,
) error {
	if signal == nil {
		return nil
	}

	switch sig := signal.GetSignal().(type) {
	case *chatv1.ClientSignal_Ack:
		return cm.processAcknowledgement(ctx, conn, sig.Ack)
	case *chatv1.ClientSignal_Typing:
		return cm.processTypingEvent(ctx, conn, sig.Typing)
	case *chatv1.ClientSignal_Receipt:
		return cm.processReceiptEvent(ctx, conn, sig.Receipt)
	case *chatv1.ClientSignal_Presence:
		return cm.processPresenceEvent(ctx, conn, sig.Presence)
	default:
		util.Log(ctx).WithField("signal_type", fmt.Sprintf("%T", signal.GetSignal())).
			Debug("Received unknown signal type")
		return nil
	}
}

// processClientCommand handles durable state changes from clients.
func (cm *connectionManager) processClientCommand(
	ctx context.Context,
	conn Connection,
	command *chatv1.ClientCommand,
) error {
	if command == nil {
		return nil
	}

	switch cmd := command.GetState().(type) {
	case *chatv1.ClientCommand_ReadMarker:
		return cm.processReadMarker(ctx, conn, cmd.ReadMarker)
	case *chatv1.ClientCommand_Event:
		return cm.processRoomEvent(ctx, conn, cmd.Event)
	default:
		util.Log(ctx).WithField("command_type", fmt.Sprintf("%T", command.GetState())).
			Debug("Received unknown command type")
		return nil
	}
}

// processAcknowledgement handles device acknowledgements for delivered messages.
func (cm *connectionManager) processAcknowledgement(
	ctx context.Context,
	conn Connection,
	ack *chatv1.EventAck,
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
			"device_id":  conn.Metadata().DeviceID,
		}).Warn("Client reported error processing event")
		return nil
	}

	// Process as a receipt event
	return cm.processReceiptEvent(ctx, conn, &chatv1.ReceiptEvent{
		Source:  &commonv1.ContactLink{ProfileId: conn.Metadata().ProfileID},
		RoomId:  "", // TODO: Extract room_id from context or event metadata
		EventId: []string{ack.GetEventId()},
	})
}

// validateAndSetProfileID validates profile ID and sets it to authenticated user.
func validateAndSetProfileID(
	ctx context.Context,
	conn Connection,
	source *commonv1.ContactLink,
	claimedProfileID, eventType string,
) error {
	profileID := conn.Metadata().ProfileID

	// Validate that the profile_id matches the authenticated user
	if claimedProfileID != "" && claimedProfileID != profileID {
		util.Log(ctx).WithFields(map[string]any{
			"claimed_profile": claimedProfileID,
			"actual_profile":  profileID,
		}).Warn("Profile ID mismatch in " + eventType + " - potential spoofing attempt")
		return fmt.Errorf(eventType+" profile_id mismatch: claimed %s, actual %s", claimedProfileID, profileID)
	}

	// Always override profile_id with authenticated profile ID for security
	if source == nil {
		source = &commonv1.ContactLink{}
	}
	source.ProfileId = profileID

	return nil
}

// processReceiptEvent handles receipt events with security validation.
func (cm *connectionManager) processReceiptEvent(
	ctx context.Context,
	conn Connection,
	receipt *chatv1.ReceiptEvent,
) error {
	if receipt == nil {
		return nil
	}

	// Validate and set profile ID
	if err := validateAndSetProfileID(ctx, conn, receipt.GetSource(), receipt.GetSource().GetProfileId(), "receipt"); err != nil {
		return err
	}

	// TODO: Implement receipt processing logic (update delivery status, etc.)
	util.Log(ctx).WithFields(map[string]any{
		"profile_id": conn.Metadata().ProfileID,
		"room_id":    receipt.GetRoomId(),
		"event_ids":  receipt.GetEventId(),
	}).Debug("Processed receipt event")

	return nil
}

// processTypingEvent handles typing indicators with security validation.
func (cm *connectionManager) processTypingEvent(
	ctx context.Context,
	conn Connection,
	typing *chatv1.TypingEvent,
) error {
	if typing == nil {
		return nil
	}

	profileID := conn.Metadata().ProfileID

	// Validate that the profile_id matches the authenticated user
	if typing.GetSource().GetProfileId() != "" && typing.GetSource().GetProfileId() != profileID {
		util.Log(ctx).WithFields(map[string]any{
			"claimed_profile": typing.GetSource().GetProfileId(),
			"actual_profile":  profileID,
			"room_id":         typing.GetRoomId(),
		}).Warn("Profile ID mismatch in typing indicator - potential spoofing attempt")
		return fmt.Errorf("typing profile_id mismatch: claimed %s, actual %s",
			typing.GetSource().GetProfileId(), profileID)
	}

	// Always override profile_id with authenticated profile ID for security
	if typing.GetSource() == nil {
		typing.Source = &commonv1.ContactLink{}
	}
	typing.Source.ProfileId = profileID

	// TODO: Implement typing indicator broadcast logic
	util.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"room_id":    typing.GetRoomId(),
	}).Debug("Processed typing event")

	return nil
}

// processPresenceEvent handles presence updates.
func (cm *connectionManager) processPresenceEvent(
	ctx context.Context,
	_ Connection,
	presence *chatv1.PresenceEvent,
) error {
	if presence == nil {
		return nil
	}

	// TODO: Implement presence event handling
	util.Log(ctx).Debug("Processed presence event")
	return nil
}

// processReadMarker handles read marker updates with security validation.
func (cm *connectionManager) processReadMarker(
	ctx context.Context,
	conn Connection,
	marker *chatv1.ReadMarker,
) error {
	if marker == nil {
		return nil
	}

	// Validate and set profile ID
	if err := validateAndSetProfileID(ctx, conn, marker.GetSource(), marker.GetSource().GetProfileId(), "read marker"); err != nil {
		return err
	}

	// TODO: Implement read marker processing logic
	util.Log(ctx).WithFields(map[string]any{
		"profile_id":     conn.Metadata().ProfileID,
		"room_id":        marker.GetRoomId(),
		"up_to_event_id": marker.GetUpToEventId(),
	}).Debug("Processed read marker")

	return nil
}

// processRoomEvent handles room events (messages) with security validation.
func (cm *connectionManager) processRoomEvent(
	ctx context.Context,
	conn Connection,
	event *chatv1.RoomEvent,
) error {
	if event == nil {
		return nil
	}

	profileID := conn.Metadata().ProfileID

	// Validate that the sender_id matches the authenticated user
	if event.GetSource().GetProfileId() != "" && event.GetSource().GetProfileId() != profileID {
		util.Log(ctx).WithFields(map[string]any{
			"claimed_sender": event.GetSource().GetProfileId(),
			"actual_sender":  profileID,
			"room_id":        event.GetRoomId(),
			"event_type":     event.GetType(),
		}).Warn("Sender ID mismatch in event - potential spoofing attempt")
		return fmt.Errorf("sender_id mismatch: claimed %s, actual %s",
			event.GetSource().GetProfileId(), profileID)
	}

	// Always override sender_id with authenticated profile ID for security
	if event.GetSource() == nil {
		event.Source = &commonv1.ContactLink{}
	}
	event.Source.ProfileId = profileID

	// Set timestamp if not provided
	if event.GetSentAt() == nil {
		event.SentAt = timestamppb.Now()
	}

	// TODO: Implement room event processing logic (validate, persist, broadcast)
	util.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"room_id":    event.GetRoomId(),
		"event_type": event.GetType(),
		"event_id":   event.GetId(),
	}).Debug("Processed room event")

	return nil
}

// DEPRECATED: oldValidationReference removed - old code structure no longer compatible with new API
// The validation logic has been split into individual handler methods above
