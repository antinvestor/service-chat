package queues

import (
	"context"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/gateway/config"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/internal"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GatewayEventsQueueHandler struct {
	cfg      *config.GatewayConfig
	qManager queue.Manager

	connectionManager business.ConnectionManager
}

func NewGatewayEventsQueueHandler(
	cfg *config.GatewayConfig,
	qManager queue.Manager,
	cm business.ConnectionManager,
) queue.SubscribeWorker {
	return &GatewayEventsQueueHandler{
		cfg:               cfg,
		qManager:          qManager,
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

	evt, err := dq.toPayloadToEventData(ctx, payload)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil
	}

	data := dq.toStreamData(evt)

	if !connection.Dispatch(data) {
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"device_id":  deviceID,
		}).Debug("dispatch channel full: slow consumer detected")

		return dq.publishToOfflineDevice(ctx, headers, evt)
	}

	return nil
}

func (dq *GatewayEventsQueueHandler) toPayloadToEventData(
	ctx context.Context,
	payload []byte,
) (*eventsv1.Delivery, error) {
	eventDelivery := &eventsv1.Delivery{}
	err := proto.Unmarshal(payload, eventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil, err
	}

	return eventDelivery, nil
}

func (dq *GatewayEventsQueueHandler) toStreamData(eventDelivery *eventsv1.Delivery) *chatv1.StreamResponse {
	evt := eventDelivery.GetEvent()

	parentID := evt.GetParentId()

	// Convert event type
	eventType := chatv1.RoomEventType(eventDelivery.GetEvent().GetEventType().Number())

	// Create RoomEvent with appropriate payload based on type
	roomEvent := &chatv1.RoomEvent{
		Id:       evt.GetEventId(),
		ParentId: &parentID,
		RoomId:   evt.GetRoomId(),
		Source:   evt.GetSource(),
		Type:     eventType,
		SentAt:   evt.GetCreatedAt(),
		Edited:   false,
		Redacted: false,
		Payload:  eventDelivery.GetPayload(),
	}

	data := &chatv1.StreamResponse{
		Id:        evt.GetEventId(),
		Timestamp: timestamppb.Now(),
		Payload: &chatv1.StreamResponse_Message{
			Message: roomEvent,
		},
	}
	return data
}

func (dq *GatewayEventsQueueHandler) getOfflineDeliveryTopic() (queue.Publisher, error) {
	deviceTopic, err := dq.qManager.GetPublisher(dq.cfg.QueueOfflineEventDeliveryName)
	if err != nil {
		return nil, err
	}

	return deviceTopic, nil
}

func (dq *GatewayEventsQueueHandler) publishToOfflineDevice(
	ctx context.Context,
	headers map[string]string,
	msg *eventsv1.Delivery,
) error {
	offlineDeliveryTopic, err := dq.getOfflineDeliveryTopic()
	if err != nil {
		return err
	}

	return offlineDeliveryTopic.Publish(ctx, msg, headers)
}
