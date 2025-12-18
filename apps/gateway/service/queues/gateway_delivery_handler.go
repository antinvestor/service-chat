package queues

import (
	"context"
	"errors"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/internal"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ErrDispatchChannelFull is returned when the connection's dispatch channel is full.
// This triggers message redelivery via the queue system.
var ErrDispatchChannelFull = errors.New("dispatch channel full: slow consumer")

type GatewayEventsQueueHandler struct {
	connectionManager business.ConnectionManager
}

func NewGatewayEventsQueueHandler(
	cm business.ConnectionManager,
) queue.SubscribeWorker {
	return &GatewayEventsQueueHandler{
		connectionManager: cm,
	}
}

func (dq *GatewayEventsQueueHandler) Handle(ctx context.Context, headers map[string]string, payload []byte) error {
	profileID := headers[internal.HeaderProfileID]
	deviceID := headers[internal.HeaderDeviceID]

	connection, ok := dq.connectionManager.GetConnection(ctx, profileID, deviceID)
	if !ok {
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Debug("connection not found: device may have disconnected")
		return nil
	}

	data, err := dq.toStreamData(ctx, payload)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil
	}

	if !connection.Dispatch(data) {
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Warn("dispatch channel full: slow consumer detected")
		// Return error to trigger message redelivery via queue system
		return ErrDispatchChannelFull
	}

	return nil
}

func (dq *GatewayEventsQueueHandler) toStreamData(
	ctx context.Context,
	payload []byte,
) (*chatv1.ConnectResponse, error) {
	eventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, eventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil, err
	}

	evt := eventDelivery.GetEvent()
	target := eventDelivery.GetTarget()

	data := &chatv1.ConnectResponse{
		Id:        target.GetTargetId(),
		Timestamp: timestamppb.Now(),
		Payload: &chatv1.ConnectResponse_Message{
			Message: &chatv1.RoomEvent{
				Id:       evt.GetEventId(),
				Type:     chatv1.RoomEventType(eventDelivery.GetEvent().GetEventType().Number()),
				Payload:  eventDelivery.GetPayload(),
				SentAt:   evt.GetCreatedAt(),
				Edited:   false,
				Redacted: false,
			},
		},
	}
	return data, nil
}
