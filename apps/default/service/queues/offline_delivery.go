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

	// Extract notification content
	messageTitle, messageBody := dq.extractMessageContent(evtMsg)

	// Convert typed payload to generic Struct for push notification data
	payloadData, err := dq.payloadToStruct(evtMsg)
	if err != nil {
		util.Log(ctx).WithError(err).Warn("failed to convert payload to struct, using empty data")
		payloadData = &structpb.Struct{}
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
			Data:   payloadData,
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

// payloadToStruct converts Delivery payload to generic Struct for push notifications.
func (dq *offlineDeliveryQueueHandler) payloadToStruct(evtMsg *eventsv1.Delivery) (*structpb.Struct, error) {
	data := make(map[string]any)

	eventPayload := evtMsg.GetPayload()
	if eventPayload == nil {
		return structpb.NewStruct(data)
	}

	switch eventPayload.GetType() {
	case chatv1.PayloadType_PAYLOAD_TYPE_TEXT:
		dq.addTextData(data, eventPayload.GetText())
	case chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT:
		dq.addAttachmentData(data, eventPayload.GetAttachment())
	case chatv1.PayloadType_PAYLOAD_TYPE_REACTION:
		dq.addReactionData(data, eventPayload.GetReaction())
	case chatv1.PayloadType_PAYLOAD_TYPE_ENCRYPTED:
		dq.addEncryptedData(data, eventPayload.GetEncrypted())
	case chatv1.PayloadType_PAYLOAD_TYPE_CALL:
		dq.addCallData(data, eventPayload.GetCall())
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION:
		dq.addMotionData(data, eventPayload.GetMotion())
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE:
		dq.addVoteData(data, eventPayload.GetVote())
	case chatv1.PayloadType_PAYLOAD_TYPE_MOTION_TALLY:
		dq.addMotionTallyData(data, eventPayload.GetMotionTally())
	case chatv1.PayloadType_PAYLOAD_TYPE_VOTE_TALLY:
		dq.addVoteTallyData(data, eventPayload.GetVoteTally())
	case chatv1.PayloadType_PAYLOAD_TYPE_UNSPECIFIED:
		// Handle unspecified payload type
	}

	return structpb.NewStruct(data)
}

// Helper methods to reduce cognitive complexity.
func (dq *offlineDeliveryQueueHandler) addTextData(data map[string]any, text *chatv1.TextContent) {
	if text != nil {
		data["text"] = text.GetBody()
		if text.GetFormat() != "" {
			data["format"] = text.GetFormat()
		}
	}
}

func (dq *offlineDeliveryQueueHandler) addAttachmentData(data map[string]any, attachment *chatv1.AttachmentContent) {
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
}

func (dq *offlineDeliveryQueueHandler) addReactionData(data map[string]any, reaction *chatv1.ReactionContent) {
	if reaction != nil {
		data["emoji"] = reaction.GetReaction()
	}
}

func (dq *offlineDeliveryQueueHandler) addEncryptedData(data map[string]any, encrypted *chatv1.EncryptedContent) {
	if encrypted != nil {
		data["ciphertext"] = string(encrypted.GetCiphertext())
		if encrypted.GetAlgorithm() != "" {
			data["algorithm"] = encrypted.GetAlgorithm()
		}
		if encrypted.SessionId != nil {
			data["sessionId"] = encrypted.GetSessionId()
		}
	}
}

func (dq *offlineDeliveryQueueHandler) addCallData(data map[string]any, call *chatv1.CallContent) {
	if call != nil && call.Sdp != nil {
		data["sdp"] = call.GetSdp()
	}
}

func (dq *offlineDeliveryQueueHandler) addMotionData(data map[string]any, motion *chatv1.MotionContent) {
	if motion != nil {
		data["id"] = motion.GetId()
		data["title"] = motion.GetTitle()
		data["description"] = motion.GetDescription()
	}
}

func (dq *offlineDeliveryQueueHandler) addVoteData(data map[string]any, vote *chatv1.VoteCast) {
	if vote != nil {
		data["motionId"] = vote.GetMotionId()
		data["choiceId"] = vote.GetChoiceId()
	}
}

func (dq *offlineDeliveryQueueHandler) addMotionTallyData(data map[string]any, motionTally *chatv1.MotionTally) {
	if motionTally != nil {
		data["motionTally"] = "completed"
	}
}

func (dq *offlineDeliveryQueueHandler) addVoteTallyData(data map[string]any, voteTally *chatv1.VoteTally) {
	if voteTally != nil {
		data["voteTally"] = "completed"
	}
}
