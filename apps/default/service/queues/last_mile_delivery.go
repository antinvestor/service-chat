package queues

import (
	"context"
	"errors"
	"io"

	devicev1 "github.com/antinvestor/apis/go/device/v1"
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

type UserDeliveryQueueHandler struct {
	service   *frame.Service
	deviceCli *devicev1.DeviceClient

	userDeviceTopic queue.Publisher
}

func NewUserDeliveryQueueHandler(
	svc *frame.Service, deviceCli *devicev1.DeviceClient,
) queue.SubscribeWorker {
	return &UserDeliveryQueueHandler{
		service:   svc,
		deviceCli: deviceCli,
	}
}

func (dq *UserDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {

	userDelivery := &eventsv1.UserDelivery{}
	err := proto.Unmarshal(payload, userDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	response, err := dq.deviceCli.Svc().Search(ctx, &devicev1.SearchRequest{
		Query: userDelivery.Target.RecepientId,
		Page:  0,
		Count: 100,
	})
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to query user devices")
		return err
	}

	for {
		resp, deviceErr := response.Recv()
		if deviceErr != nil {

			if !errors.Is(deviceErr, io.EOF) {
				util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
			}
			break
		}

		dq.deliverMessageToDevice(ctx, userDelivery, resp.Data)
	}
	return nil
}

func (dq *UserDeliveryQueueHandler) deliverMessageToDevice(ctx context.Context, msg *eventsv1.UserDelivery, devices []*devicev1.DeviceObject) {

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

func (dq *UserDeliveryQueueHandler) deviceIsOnline(ctx context.Context, dev *devicev1.DeviceObject) bool {
	status := dev.GetPresence()
	if devicev1.PresenceStatus_OFFLINE == status {
		return false
	}
	return true
}

func (dq *UserDeliveryQueueHandler) publishToDevice(ctx context.Context, dev *devicev1.DeviceObject, msg *eventsv1.UserDelivery) error {

	deviceHeader := map[string]string{
		HeaderProfileID: msg.GetTarget().GetRecepientId(),
		HeaderDeviceID:  dev.GetId(),
	}

	return dq.userDeviceTopic.Publish(ctx, msg, deviceHeader)
}

func (dq *UserDeliveryQueueHandler) publishToFCM(ctx context.Context, dev *devicev1.DeviceObject, msg *eventsv1.UserDelivery) error {

	fields := msg.GetPayload().GetFields()

	messageTitle := "Stawi message"
	title, ok := fields["title"]
	if ok {
		messageTitle = title.String()
	}
	messageBody := ""
	body, ok := fields["content"]
	if ok && msg.GetEvent().GetEventType() == eventsv1.ChatEvent_TEXT {
		messageBody = body.String()
	}

	extraData, err := structpb.NewStruct(map[string]any{
		"event":  msg.GetEvent(),
		"target": msg.GetTarget(),
	})

	if err != nil {
		return err
	}

	notification := &devicev1.NotifyRequest{
		DeviceId: dev.GetId(),
		KeyType:  devicev1.KeyType_FCM_TOKEN,
		Title:    messageTitle,
		Body:     messageBody,
		Data:     msg.GetPayload(),
		Extras:   extraData,
	}

	resp, err := dq.deviceCli.Svc().Notify(ctx, notification)
	if err != nil {
		return err
	}

	util.Log(ctx).WithField("resp", resp).Debug("fcm notification response successful")

	return nil
}
