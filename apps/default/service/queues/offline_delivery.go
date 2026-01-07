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
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type offlineDeliveryQueueHandler struct {
	cfg       *config.ChatConfig
	deviceCli devicev1connect.DeviceServiceClient

	payloadConverter *models.PayloadConverter
}

func NewOfflineDeliveryQueueHandler(
	cfg *config.ChatConfig,
	deviceCli devicev1connect.DeviceServiceClient,
) queue.SubscribeWorker {
	return &offlineDeliveryQueueHandler{
		cfg:       cfg,
		deviceCli: deviceCli,

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
	messageTitle, messageBody := dq.extractMessageContent(evtMsg)

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
	resp, err := dq.deviceCli.Notify(ctx, connect.NewRequest(notification))
	if err != nil {
		return err
	}

	util.Log(ctx).WithField("resp", resp).Debug("fcm notification response successful")

	return nil
}

// extractMessageContent extracts notification content from typed payload.
func (dq *offlineDeliveryQueueHandler) extractMessageContent(evtMsg *eventsv1.Delivery) (string, string) {
	messageTitle := "Stawi message"
	messageBody := ""

	eventPayload := evtMsg.GetPayload()
	if eventPayload != nil {
		switch eventPayload.GetType() {
		case chatv1.PayloadType_PAYLOAD_TYPE_MODERATION:
			evt := eventPayload.GetModeration()
			if evt != nil {
				messageBody = evt.GetBody()
			}
		case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
			text := eventPayload.GetText()
			if text != nil {
				messageBody = text.GetBody()
			}
		case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
			messageBody = "Sent an attachment"
			attachment := eventPayload.GetAttachment()
			if attachment != nil && attachment.GetCaption() != nil {
				messageBody = attachment.GetCaption().GetBody()
			}
		case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
			reaction := eventPayload.GetReaction()
			if reaction != nil {
				messageBody = "Reacted with " + reaction.GetReaction()
			}
		case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
			messageBody = "Sent an encrypted message"
		case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
			messageBody = "Started a call"
		case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
			messageBody = "Created a motion"
		case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
			messageBody = "Voted"
		case chatv1.PayloadType_PAYLOAD_TYPE_MOTION_TALLY:
			messageBody = "Motion tally completed"
		case chatv1.PayloadType_PAYLOAD_TYPE_VOTE_TALLY:
			messageBody = "Vote tally completed"
		case chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED:
			// Handle unspecified payload type
		}
	}

	return messageTitle, messageBody
}
