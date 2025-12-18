package queues

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/internal"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	// DeviceSearchPageSize defines the number of devices to fetch per page when searching.
	DeviceSearchPageSize        = 100
	DeliveryRequestReplyTimeout = 10 * time.Second
)

type EventDeliveryQueueHandler struct {
	qMan      queue.Manager
	cfg       *config.ChatConfig
	deviceCli devicev1connect.DeviceServiceClient
}

func NewEventDeliveryQueueHandler(
	cfg *config.ChatConfig,
	qMan queue.Manager,
	deviceCli devicev1connect.DeviceServiceClient,
) queue.SubscribeWorker {

	return &EventDeliveryQueueHandler{
		cfg:       cfg,
		qMan:      qMan,
		deviceCli: deviceCli,
	}
}

func (dq *EventDeliveryQueueHandler) getTopic(profileID, deviceID string) (queue.Publisher, int, error) {

	shardString := internal.MetadataKey(profileID, deviceID)
	shardID := internal.ShardForKey(shardString, uint32(dq.cfg.ShardCount))

	shardDeliveryQueueName := fmt.Sprintf(dq.cfg.QueueDeviceEventDeliveryName, shardID)

	deviceTopic, err := dq.qMan.GetPublisher(shardDeliveryQueueName)
	if err != nil {
		return nil, shardID, err
	}

	return deviceTopic, shardID, nil
}

func (dq *EventDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	eventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, eventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	response, err := dq.deviceCli.Search(ctx, connect.NewRequest(&devicev1.SearchRequest{
		Query: eventDelivery.GetTarget().GetRecepientId(),
		Page:  0,
		Count: DeviceSearchPageSize,
	}))
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to query user devices")
		return err
	}

	for response.Receive() {

		deviceErr := response.Err()
		if deviceErr != nil {
			if !errors.Is(deviceErr, io.EOF) {
				util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
			}
		}

		resp := response.Msg()
		dq.deliverMessageToDevice(ctx, eventDelivery, resp.GetData())
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
			util.Log(ctx).WithError(err).Error("directly delivery of message failed")
		}

		err := dq.publishToFCM(ctx, dev, msg)
		if err != nil {
			util.Log(ctx).WithError(err).Error("deliver of message via FCM failed")
		}
	}
}

func (dq *EventDeliveryQueueHandler) deviceIsOnline(_ context.Context, dev *devicev1.DeviceObject) bool {
	return dev.GetPresence() != devicev1.PresenceStatus_OFFLINE
}

func (dq *EventDeliveryQueueHandler) publishToDevice(
	ctx context.Context,
	dev *devicev1.DeviceObject,
	msg *eventsv1.EventDelivery,
) error {

	profileID := msg.GetTarget().GetRecepientId()
	deviceID := dev.GetId()

	deliveryTopic, shardID, err := dq.getTopic(profileID, deviceID)
	if err != nil {
		return err
	}

	deviceHeader := map[string]string{
		internal.HeaderProfileID: profileID,
		internal.HeaderDeviceID:  deviceID,
		internal.HeaderShardID:   strconv.Itoa(shardID),
	}

	return deliveryTopic.Publish(ctx, msg, deviceHeader)
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
