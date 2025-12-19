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
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/proto"
)

const (
	// DeviceSearchPageSize defines the number of devices to fetch per page when searching.
	DeviceSearchPageSize        = 100
	DeliveryRequestReplyTimeout = 10 * time.Second
)

type hotPathDeliveryQueueHandler struct {
	qMan      queue.Manager
	workMan   workerpool.Manager
	cfg       *config.ChatConfig
	deviceCli devicev1connect.DeviceServiceClient
}

func NewHotPathDeliveryQueueHandler(
	cfg *config.ChatConfig,
	qMan queue.Manager,
	workMan workerpool.Manager,
	deviceCli devicev1connect.DeviceServiceClient,
) queue.SubscribeWorker {
	return &hotPathDeliveryQueueHandler{
		cfg:       cfg,
		qMan:      qMan,
		workMan:   workMan,
		deviceCli: deviceCli,
	}
}

func (dq *hotPathDeliveryQueueHandler) getOfflineDeliveryTopic() (queue.Publisher, error) {
	deviceTopic, err := dq.qMan.GetPublisher(dq.cfg.QueueOfflineEventDeliveryName)
	if err != nil {
		return nil, err
	}

	return deviceTopic, nil
}

func (dq *hotPathDeliveryQueueHandler) getOnlineDeliveryTopic(
	ctx context.Context,
	profileID, deviceID string,
) (queue.Publisher, int, error) {
	shardString := internal.MetadataKey(profileID, deviceID)

	// Ensure ShardCount is valid
	if dq.cfg.ShardCount <= 0 {
		util.Log(ctx).WithField("shard_count", dq.cfg.ShardCount).
			Error("Invalid shard count, must be positive")
		return nil, 0, fmt.Errorf("invalid shard count: %d", dq.cfg.ShardCount)
	}

	shardID := internal.ShardForKey(shardString, dq.cfg.ShardCount)

	shardDeliveryQueueName := fmt.Sprintf(dq.cfg.QueueDeviceEventDeliveryName, shardID)

	deviceTopic, err := dq.qMan.GetPublisher(shardDeliveryQueueName)
	if err != nil {
		return nil, shardID, err
	}

	return deviceTopic, shardID, nil
}

func (dq *hotPathDeliveryQueueHandler) Handle(ctx context.Context, _ map[string]string, payload []byte) error {
	eventDelivery := &eventsv1.EventDelivery{}
	err := proto.Unmarshal(payload, eventDelivery)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to unmarshal user delivery")
		return err
	}

	response, err := dq.deviceCli.Search(ctx, connect.NewRequest(&devicev1.SearchRequest{
		Query: eventDelivery.GetRecepientId(),
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

		// Process devices concurrently for faster delivery
		for _, dev := range resp.GetData() {
			job := workerpool.NewJob[any](func(ctx context.Context, resultPipe workerpool.JobResultPipe[any]) error {
				eventCopy := eventDelivery
				eventCopy.DeviceId = dev.GetId()

				deliveryErr := dq.deliver(ctx, eventCopy, dev)
				if deliveryErr != nil {
					util.Log(ctx).WithError(deliveryErr).WithField("device_id", dev.GetId()).
						Error("failed to deliver event")
					return resultPipe.WriteError(ctx, deliveryErr)
				}
				return nil
			})

			err = workerpool.SubmitJob(ctx, dq.workMan, job)
			if err != nil {
				util.Log(ctx).WithError(err).WithField("device_id", dev.GetId()).
					Error("failed to submit job")
			}
		}
	}

	return nil
}

func (dq *hotPathDeliveryQueueHandler) deliver(
	ctx context.Context,
	msg *eventsv1.EventDelivery,
	dev *devicev1.DeviceObject,
) error {
	if dq.deviceIsOnline(ctx, dev) {
		err := dq.publishToOnlineDevice(ctx, dev, msg)
		if err == nil {
			return nil
		}
		util.Log(ctx).WithError(err).WithField("device_id", dev.GetId()).
			Debug("direct delivery failed, falling back to offline delivery")
	}

	offlineDeliveryTopic, err := dq.getOfflineDeliveryTopic()
	if err != nil {
		return err
	}

	deviceHeader := map[string]string{
		internal.HeaderDeviceID: dev.GetId(),
	}

	return offlineDeliveryTopic.Publish(ctx, msg, deviceHeader)
}

func (dq *hotPathDeliveryQueueHandler) deviceIsOnline(_ context.Context, dev *devicev1.DeviceObject) bool {
	return dev.GetPresence() != devicev1.PresenceStatus_OFFLINE
}

func (dq *hotPathDeliveryQueueHandler) publishToOnlineDevice(
	ctx context.Context,
	dev *devicev1.DeviceObject,
	msg *eventsv1.EventDelivery,
) error {
	profileID := msg.GetRecepientId()
	deviceID := dev.GetId()

	deliveryTopic, shardID, err := dq.getOnlineDeliveryTopic(ctx, profileID, deviceID)
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
