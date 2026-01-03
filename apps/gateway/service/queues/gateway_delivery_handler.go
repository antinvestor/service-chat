package queues

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/gateway/config"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/internal"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
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
) (*eventsv1.EventDelivery, error) {
	eventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, eventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("Failed to parse user delivery message")
		return nil, err
	}

	return eventDelivery, nil
}

func (dq *GatewayEventsQueueHandler) toStreamData(eventDelivery *eventsv1.EventDelivery) *chatv1.StreamResponse {
	evt := eventDelivery.GetEvent()

	parentID := evt.GetParentId()

	// Convert event type
	eventType := chatv1.RoomEventType(eventDelivery.GetEvent().GetEventType().Number())

	// Create RoomEvent with appropriate payload based on type
	roomEvent := &chatv1.RoomEvent{
		Id:       evt.GetEventId(),
		ParentId: &parentID,
		RoomId:   evt.GetRoomId(),
		SenderId: evt.GetSenderId(),
		Type:     eventType,
		SentAt:   evt.GetCreatedAt(),
		Edited:   false,
		Redacted: false,
	}

	// Convert generic payload to typed payload based on event type
	payload := eventDelivery.GetPayload()
	if payload != nil {
		switch eventType {
		case chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT:
			// Extract text content from generic payload
			if body, ok := payload.GetFields()["body"]; ok && body.GetStringValue() != "" {
				roomEvent.Payload = &chatv1.RoomEvent_Text{
					Text: &chatv1.TextContent{
						Body: body.GetStringValue(),
					},
				}
			}
			// For other event types, we don't set a payload for now
			// TODO: Handle ATTACHMENT, REACTION, ENCRYPTED, CALL types when needed
		}
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
	msg *eventsv1.EventDelivery,
) error {
	offlineDeliveryTopic, err := dq.getOfflineDeliveryTopic()
	if err != nil {
		return err
	}

	return offlineDeliveryTopic.Publish(ctx, msg, headers)
}
