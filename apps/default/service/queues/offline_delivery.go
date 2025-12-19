package queues

import (
	"context"

	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/config"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
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
	evtMsg := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, evtMsg)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	fields := evtMsg.GetPayload().GetFields()

	messageTitle := "Stawi message"
	title, ok := fields["title"]
	if ok {
		messageTitle = title.String()
	}
	messageBody := ""
	body, ok := fields["content"]
	if ok && evtMsg.GetEvent().GetEventType() == eventsv1.EventLink_TEXT {
		messageBody = body.String()
	}

	extraData, err := structpb.NewStruct(map[string]any{
		"event":  evtMsg.GetEvent(),
		"target": evtMsg.GetRecepientId(),
	})

	if err != nil {
		return err
	}

	messages := []*devicev1.NotifyMessage{
		{
			Title:  messageTitle,
			Body:   messageBody,
			Data:   evtMsg.GetPayload(),
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
