package business

import (
	"context"
	"errors"
	"fmt"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/util"
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
	log := util.Log(ctx).With(
		"profile_id", conn.Metadata().ProfileID,
		"device_id", conn.Metadata().DeviceID,
	)

	if !conn.AllowInbound() {
		log.Warn("Request rate limited")
		return ErrRateLimited
	}

	if req == nil {
		log.Warn("Received nil request")
		return nil
	}

	switch cmd := req.GetPayload().(type) {
	case *chatv1.StreamRequest_Command:
		if cmd.Command == nil {
			log.Warn("Received nil command")
			return nil
		}
		return cm.processClientCommand(ctx, cmd.Command)
	case *chatv1.StreamRequest_Hello:
		log.Debug("Received Hello message after connection established")
		return nil
	default:
		log.With("payload_type", fmt.Sprintf("%T", req.GetPayload())).
			Debug("Received unknown payload type")
		return nil
	}
}

// processClientCommand handles durable state changes from clients.
func (cm *connectionManager) processClientCommand(
	ctx context.Context,
	command *chatv1.ClientCommand,
) error {
	if command == nil {
		util.Log(ctx).Warn("Received nil command")
		return nil
	}

	switch cmd := command.GetState().(type) {
	case *chatv1.ClientCommand_ReadMarker:
		return cm.processReadMarker(ctx, cmd.ReadMarker)
	case *chatv1.ClientCommand_Event:
		return cm.processRoomEvent(ctx, cmd.Event)
	case *chatv1.ClientCommand_Ack:
		return cm.processAcknowledgement(ctx, cmd.Ack)
	case *chatv1.ClientCommand_Typing:
		return cm.processTypingEvent(ctx, cmd.Typing)
	case *chatv1.ClientCommand_Receipt:
		return cm.processReceiptEvent(ctx, cmd.Receipt)
	case *chatv1.ClientCommand_Presence:
		return cm.processPresenceEvent(ctx, cmd.Presence)
	default:
		util.Log(ctx).WithField("command_type", fmt.Sprintf("%T", command.GetState())).
			Debug("Received unknown command type")
		return nil
	}
}

// processAcknowledgement handles device acknowledgements for delivered messages.
func (cm *connectionManager) processAcknowledgement(
	ctx context.Context,
	ack *chatv1.AckEvent,
) error {
	if ack == nil {
		util.Log(ctx).With(
			"event_id", ack.GetEventId(),
		).Warn("Received nil ack event")
		return nil
	}

	// If there's an error in the ack, log it for debugging
	if ack.GetError() != nil {
		err := ack.GetError()
		util.Log(ctx).With(
			"event_id", ack.GetEventId(),
		).Warn("Client reported error processing event",
			"error_code", err.GetCode(),
			"error_msg", err.GetMessage(),
		)
	}

	// Only process as receipt if there's no error
	if ack.GetError() == nil {
		// Process as a receipt event
		return cm.processReceiptEvent(ctx, &chatv1.ReceiptEvent{
			RoomId:         ack.GetRoomId(),
			SubscriptionId: ack.GetSubscriptionId(),
			EventId:        ack.GetEventId(),
		})
	}

	return nil
}

// processReceiptEvent handles receipt events with security validation.
func (cm *connectionManager) processReceiptEvent(
	ctx context.Context,
	receipt *chatv1.ReceiptEvent,
) error {
	if receipt == nil {
		util.Log(ctx).With(
			"event_id", receipt.GetEventId(),
			"room_id", receipt.GetRoomId(),
		).Warn("Received nil receipt event")
		return nil
	}

	return cm.processLiveRequest(ctx, &chatv1.ClientCommand{
		State: &chatv1.ClientCommand_Receipt{Receipt: receipt},
	})
}

// processTypingEvent handles typing indicators with security validation.
func (cm *connectionManager) processTypingEvent(
	ctx context.Context,
	typing *chatv1.TypingEvent,
) error {
	if typing == nil {
		util.Log(ctx).With(
			"room_id", typing.GetRoomId(),
			"typing", typing.GetTyping(),
		).Warn("Received nil typing event")
		return nil
	}

	return cm.processLiveRequest(ctx, &chatv1.ClientCommand{
		State: &chatv1.ClientCommand_Typing{Typing: typing},
	})
}

// processPresenceEvent handles presence updates.
func (cm *connectionManager) processPresenceEvent(
	ctx context.Context,
	presence *chatv1.PresenceEvent,
) error {
	if presence == nil {
		util.Log(ctx).Warn("Received nil presence event")
		return nil
	}

	return cm.processLiveRequest(ctx, &chatv1.ClientCommand{
		State: &chatv1.ClientCommand_Presence{Presence: presence},
	})
}

// processReadMarker handles read marker updates with security validation.
func (cm *connectionManager) processReadMarker(
	ctx context.Context,
	marker *chatv1.ReadMarker,
) error {
	if marker == nil {
		return nil
	}

	return cm.processLiveRequest(ctx, &chatv1.ClientCommand{
		State: &chatv1.ClientCommand_ReadMarker{ReadMarker: marker}})
}

// processRoomEvent handles room events (messages) with security validation.
func (cm *connectionManager) processRoomEvent(
	ctx context.Context,
	event *chatv1.RoomEvent,
) error {
	if event == nil {
		util.Log(ctx).With(
			"event_id", event.GetId(),
			"room_id", event.GetRoomId(),
			"event_type", event.GetType(),
		).Warn("Received nil room event")
		return nil
	}

	return cm.processLiveRequest(ctx, &chatv1.ClientCommand{
		State: &chatv1.ClientCommand_Event{Event: event},
	})
}

// processLiveRequest is a helper function to process live requests with common error handling and logging.
func (cm *connectionManager) processLiveRequest(
	ctx context.Context,
	command *chatv1.ClientCommand,
) error {
	if command == nil {
		util.Log(ctx).Warn("Received nil command for live request")
		return nil
	}

	source, err := internal.AuthContactLink(ctx)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to get authenticated contact link")
		return fmt.Errorf("failed to get contact link: %w", err)
	}

	// Create and send the live request
	resp, err := cm.chatClient.Live(ctx, connect.NewRequest(&chatv1.LiveRequest{
		Source:       source,
		ClientStates: []*chatv1.ClientCommand{command},
	}))

	// Handle any errors from the live call
	if err != nil {
		errMsg := "Failed to process live request"
		util.Log(ctx).WithError(err).WithField("command_type", fmt.Sprintf("%T", command.GetState())).Error(errMsg)
		return fmt.Errorf("%s: %w", errMsg, err)
	}

	clErr := resp.Msg.GetError()
	if clErr != nil {
		util.Log(ctx).
			WithField("code", clErr.GetCode()).
			WithField("message", clErr.GetMessage()).
			Error("live request error")
	}

	return nil
}
