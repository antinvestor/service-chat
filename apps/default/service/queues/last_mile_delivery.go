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

type UserDeliveryQueueHandler struct {
	service   *frame.Service
	deviceCli *devicev1.DeviceClient
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

}
