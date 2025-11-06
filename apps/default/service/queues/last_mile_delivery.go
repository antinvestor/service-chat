package queues

import (
	"context"
	"errors"
	"io"

	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	HeaderPriority  = "priority"
	HeaderProfileID = "profile_id"
	HeaderDeviceID  = "device_id"
)

type EventDeliveryQueueHandler struct {
	service   *frame.Service
	deviceCli devicev1connect.DeviceServiceClient

	userDeviceTopic queue.Publisher
}

func NewEventDeliveryQueueHandler(
	svc *frame.Service, deviceCli devicev1connect.DeviceServiceClient,
) queue.SubscribeWorker {
	return &EventDeliveryQueueHandler{
		service:   svc,
		deviceCli: deviceCli,
	}
}

func (dq *EventDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	EventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, EventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	response, err := dq.deviceCli.Search(ctx, connect.NewRequest(&devicev1.SearchRequest{
		Query: EventDelivery.GetTarget().GetRecepientId(),
		Page:  0,
		Count: 100,
	}))
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to query user devices")
		return err
	}

	for response.Receive() {
		resp := response.Msg()
		dq.deliverMessageToDevice(ctx, EventDelivery, resp.GetData())
	}

	if deviceErr := response.Err(); deviceErr != nil {
		if !errors.Is(deviceErr, io.EOF) {
			util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		}
	}

	return nil
}

func (dq *EventDeliveryQueueHandler) deliverMessageToDevice(
	ctx context.Context,
	msg *eventsv1.EventDelivery,
	devices []*devicev1.DeviceObject,
) {
	for _, dev := range devices {
		if dq.deviceIsOnline(ctx, dev) {
			err := dq.publishToDevice(ctx, dev, msg)
			if err == nil {
				continue
			}
			util.Log(ctx).WithError(err).Error("failed to directly deliver message")
		}

		err := dq.publishToFCM(ctx, dev, msg)
		if err != nil {
			util.Log(ctx).WithError(err).Error("failed to deliver message via FCM")
		}
	}
}

func (dq *EventDeliveryQueueHandler) deviceIsOnline(ctx context.Context, dev *devicev1.DeviceObject) bool {
	status := dev.GetPresence()
	if devicev1.PresenceStatus_OFFLINE == status {
		return false
	}
	return true
}

func (dq *EventDeliveryQueueHandler) publishToDevice(
	ctx context.Context,
	dev *devicev1.DeviceObject,
	msg *eventsv1.EventDelivery,
) error {
	deviceHeader := map[string]string{
		HeaderProfileID: msg.GetTarget().GetRecepientId(),
		HeaderDeviceID:  dev.GetId(),
	}

	return dq.userDeviceTopic.Publish(ctx, msg, deviceHeader)
}

func (dq *EventDeliveryQueueHandler) publishToFCM(
	ctx context.Context,
	dev *devicev1.DeviceObject,
	msgList ...*eventsv1.EventDelivery,
) error {
	var messages []*devicev1.NotifyMessage

	for _, msg := range msgList {
		fields := msg.GetPayload().GetFields()

		messageTitle := "Stawi message"
		title, ok := fields["title"]
		if ok {
			messageTitle = title.String()
		}
		messageBody := ""
		body, ok := fields["content"]
		if ok && msg.GetEvent().GetEventType() == eventsv1.EventLink_TEXT {
			messageBody = body.String()
		}

		extraData, err := structpb.NewStruct(map[string]any{
			"event":  msg.GetEvent(),
			"target": msg.GetTarget(),
		})

		if err != nil {
			return err
		}

		messages = append(messages, &devicev1.NotifyMessage{
			Title:  messageTitle,
			Body:   messageBody,
			Data:   msg.GetPayload(),
			Extras: extraData,
		})
	}

	notification := &devicev1.NotifyRequest{
		DeviceId:      dev.GetId(),
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
