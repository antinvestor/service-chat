package queues

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/internal"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

	data, err := dq.toStreamData(ctx, payload)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil
	}

	profileID := headers[internal.HeaderProfileID]
	deviceID := headers[internal.HeaderDeviceID]

	connection, ok := dq.connectionManager.GetConnection(ctx, profileID, deviceID)
	if !ok {
		util.Log(ctx).Error("connection not found: probably subscribed elsewhere")
		return nil
	}

	connection.Dispatch(data)

	return nil
}

func (dq *GatewayEventsQueueHandler) toStreamData(ctx context.Context, payload []byte) (*chatv1.ConnectResponse, error) {

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
