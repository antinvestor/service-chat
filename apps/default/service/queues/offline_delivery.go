package queues

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/internal/resilience"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type offlineDeliveryQueueHandler struct {
	cfg       *config.ChatConfig
	deviceCli devicev1connect.DeviceServiceClient
	deviceCB  *resilience.CircuitBreaker

	payloadConverter *models.PayloadConverter
}

func NewOfflineDeliveryQueueHandler(
	cfg *config.ChatConfig,
	deviceCli devicev1connect.DeviceServiceClient,
	deviceCB *resilience.CircuitBreaker,
) queue.SubscribeWorker {
	return &offlineDeliveryQueueHandler{
		cfg:       cfg,
		deviceCli: deviceCli,
		deviceCB:  deviceCB,

		payloadConverter: models.NewPayloadConverter(),
	}
}

func (dq *offlineDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	evtMsg := &eventsv1.Delivery{}
	err := proto.Unmarshal(payload, evtMsg)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	// Extract notification content
	messageTitle := "Stawi message"
	messageBody := dq.extractMessageBody(evtMsg)

	// Convert typed payload to generic Struct for push notification data
	payloadData, err := dq.payloadConverter.FromProto(evtMsg.GetPayload())
	if err != nil {
		util.Log(ctx).WithError(err).Warn("failed to convert payload to struct, using empty data")
		payloadData = data.JSONMap{}
	}

	destination := evtMsg.GetDestination()
	targetID := ""
	if destination != nil {
		contactLink := destination.GetContactLink()
		if contactLink != nil {
			targetID = contactLink.GetProfileId()
		}
	}

	if targetID == "" {
		util.Log(ctx).Warn("target profile ID is empty, skipping notification")
		return nil
	}

	// Create push notification message
	messages := []*devicev1.NotifyMessage{
		{
			Title:  messageTitle,
			Body:   messageBody,
			Data:   payloadData.ToProtoStruct(),
			Extras: &structpb.Struct{},
		},
	}

	notification := &devicev1.NotifyRequest{
		DeviceId:      evtMsg.GetDeviceId(),
		KeyType:       devicev1.KeyType_FCM_TOKEN,
		Notifications: messages,
	}
	var resp *connect.Response[devicev1.NotifyResponse]
	notifyFn := func() error {
		var notifyErr error
		resp, notifyErr = dq.deviceCli.Notify(ctx, connect.NewRequest(notification))
		return notifyErr
	}

	if dq.deviceCB != nil {
		err = dq.deviceCB.Execute(notifyFn)
	} else {
		err = notifyFn()
	}
	if err != nil {
		return err
	}

	util.Log(ctx).WithField("resp", resp).Debug("fcm notification response successful")

	return nil
}

// extractMessageBody extracts the notification body from typed payload.
func (dq *offlineDeliveryQueueHandler) extractMessageBody(evtMsg *eventsv1.Delivery) string {
	eventPayload := evtMsg.GetPayload()
	if eventPayload == nil {
		return ""
	}
	return dq.bodyFromPayload(eventPayload)
}

// bodyFromPayload extracts a human-readable body string from a typed payload.
func (dq *offlineDeliveryQueueHandler) bodyFromPayload(eventPayload *chatv1.Payload) string {
	switch eventPayload.GetType() {
	case chatv1.PayloadType_PAYLOAD_TYPE_MODERATION:
		if evt := eventPayload.GetModeration(); evt != nil {
			return evt.GetBody()
		}
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		if text := eventPayload.GetText(); text != nil {
			return text.GetBody()
		}
	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		if attachment := eventPayload.GetAttachment(); attachment != nil && attachment.GetCaption() != nil {
			return attachment.GetCaption().GetBody()
		}
		return "Sent an attachment"
	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		if reaction := eventPayload.GetReaction(); reaction != nil {
			return "Reacted with " + reaction.GetReaction()
		}
	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		return "Sent an encrypted message"
	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		return "Started a call"
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		return "Created a motion"
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		return "Voted"
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION_TALLY:
		return "Motion tally completed"
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE_TALLY:
		return "Vote tally completed"
	case chatv1.PayloadType_PAYLOAD_TYPE_ROOM_CHANGE:
		if roomChange := eventPayload.GetRoomChange(); roomChange != nil {
			return roomChange.GetBody()
		}
	case chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED:
		// No body for unspecified type
	}
	return ""
}
