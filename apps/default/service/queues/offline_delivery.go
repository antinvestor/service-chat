package queues

import (
	"context"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

type offlineDeliveryQueueHandler struct {
	cfg       *config.ChatConfig
	deviceCli devicev1connect.DeviceServiceClient
}

func NewOfflineDeliveryQueueHandler(
	cfg *config.ChatConfig,
	deviceCli devicev1connect.DeviceServiceClient,
) queue.SubscribeWorker {
	return &offlineDeliveryQueueHandler{
		cfg:       cfg,
		deviceCli: deviceCli,
	}
}

func (dq *offlineDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	evtMsg := &eventsv1.Delivery{}
	err := proto.Unmarshal(payload, evtMsg)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	// Extract notification content from typed payload
	messageTitle := "Stawi message"
	messageBody := ""

	eventPayload := evtMsg.GetPayload()
	if eventPayload != nil {
		switch eventPayload.GetType() {
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
		}
	}

	// Convert typed payload to generic Struct for push notification data
	payloadData, err := dq.payloadToStruct(evtMsg)
	if err != nil {
		util.Log(ctx).WithError(err).Warn("failed to convert payload to struct, using empty data")
		payloadData = &structpb.Struct{}
	}

	destination := evtMsg.GetDestination()
	targetID := ""
	if destination != nil {
		targetID = destination.GetProfileId()
	}

	extraData, err := structpb.NewStruct(map[string]any{
		"event":  evtMsg.GetEvent(),
		"target": targetID,
	})

	if err != nil {
		return err
	}

	messages := []*devicev1.NotifyMessage{
		{
			Title:  messageTitle,
			Body:   messageBody,
			Data:   payloadData,
			Extras: extraData,
		}}

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

// payloadToStruct converts Delivery payload to generic Struct for push notifications.
func (dq *offlineDeliveryQueueHandler) payloadToStruct(evtMsg *eventsv1.Delivery) (*structpb.Struct, error) {
	data := make(map[string]any)

	eventPayload := evtMsg.GetPayload()
	if eventPayload == nil {
		return structpb.NewStruct(data)
	}

	switch eventPayload.GetType() {
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		text := eventPayload.GetText()
		if text != nil {
			data["text"] = text.GetBody()
			if text.GetFormat() != "" {
				data["format"] = text.GetFormat()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		attachment := eventPayload.GetAttachment()
		if attachment != nil {
			data["attachmentId"] = attachment.GetAttachmentId()
			if attachment.GetFilename() != "" {
				data["fileName"] = attachment.GetFilename()
			}
			if attachment.GetMimeType() != "" {
				data["mimeType"] = attachment.GetMimeType()
			}
			if attachment.GetSizeBytes() != 0 {
				data["size"] = attachment.GetSizeBytes()
			}
			if attachment.GetCaption() != nil {
				data["caption"] = attachment.GetCaption().GetBody()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		reaction := eventPayload.GetReaction()
		if reaction != nil {
			data["emoji"] = reaction.GetReaction()
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		encrypted := eventPayload.GetEncrypted()
		if encrypted != nil {
			data["ciphertext"] = string(encrypted.GetCiphertext())
			if encrypted.GetAlgorithm() != "" {
				data["algorithm"] = encrypted.GetAlgorithm()
			}
			if encrypted.SessionId != nil {
				data["sessionId"] = encrypted.GetSessionId()
			}
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		call := eventPayload.GetCall()
		if call != nil && call.Sdp != nil {
			data["sdp"] = call.GetSdp()
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		motion := eventPayload.GetMotion()
		if motion != nil {
			data["id"] = motion.GetId()
			data["title"] = motion.GetTitle()
			data["description"] = motion.GetDescription()
		}

	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		vote := eventPayload.GetVote()
		if vote != nil {
			data["motionId"] = vote.GetMotionId()
			data["choiceId"] = vote.GetChoiceId()
		}
	}

	return structpb.NewStruct(data)
}
