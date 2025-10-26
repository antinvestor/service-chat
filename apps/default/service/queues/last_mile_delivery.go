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
)

const (
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
	// TODO: Implement actual online detection by checking gateway connection cache
	// For now, assume device is online and let gateway handle offline fallback
	return true
}

func (dq *UserDeliveryQueueHandler) publishToDevice(ctx context.Context, dev *devicev1.DeviceObject, msg *eventsv1.UserDelivery) error {

	deviceHeader := map[string]string{
		HeaderProfileID: msg.Target.GetRecepientId(),
		HeaderDeviceID:  dev.GetId(),
	}

	return dq.userDeviceTopic.Publish(ctx, msg, deviceHeader)
}

func (dq *UserDeliveryQueueHandler) publishToFCM(ctx context.Context, dev *devicev1.DeviceObject, msg *eventsv1.UserDelivery) error {
	// TODO: Implement push notification delivery for offline devices
	// This should integrate with a notification service (not FCM directly)
	// The notification service will handle FCM, APNs, etc.

	util.Log(ctx).WithFields(map[string]any{
		"device_id":  dev.GetId(),
		"profile_id": msg.GetTarget().GetRecepientId(),
		"event_id":   msg.GetEvent().GetEventId(),
	}).Info("TODO: send push notification for offline device")

	return nil
}
