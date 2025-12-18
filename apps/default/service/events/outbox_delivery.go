package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const RoomOutboxDeliveryEventName = "outbox.delivery.event"

type OutboxDeliveryEventHandler struct {
	cfg           *config.ChatConfig
	eventRepo     repository.RoomEventRepository
	queueMan      queue.Manager
	deliveryTopic queue.Publisher
}

func NewOutboxDeliveryEventHandler(
	ctx context.Context,
	cfg *config.ChatConfig,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	queueMan queue.Manager,
) *OutboxDeliveryEventHandler {
	return &OutboxDeliveryEventHandler{
		cfg:       cfg,
		queueMan:  queueMan,
		eventRepo: repository.NewRoomEventRepository(ctx, dbPool, workMan),
	}
}

func (dlrEH *OutboxDeliveryEventHandler) getTopic() (queue.Publisher, error) {
	if dlrEH.deliveryTopic != nil {
		return dlrEH.deliveryTopic, nil
	}

	var err error
	dlrEH.deliveryTopic, err = dlrEH.queueMan.GetPublisher(dlrEH.cfg.QueueDeviceEventDeliveryName)
	if err != nil {
		return nil, err
	}
	return dlrEH.deliveryTopic, nil
}

func (dlrEH *OutboxDeliveryEventHandler) Name() string {
	return RoomOutboxDeliveryEventName
}

func (dlrEH *OutboxDeliveryEventHandler) PayloadType() any {
	return &eventsv1.EventBroadcast{}
}

func (dlrEH *OutboxDeliveryEventHandler) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.EventBroadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.EventBroadcast")
	}
	return nil
}

func (dlrEH *OutboxDeliveryEventHandler) Execute(ctx context.Context, payload any) error {
	broadcast, ok := payload.(*eventsv1.EventBroadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.EventBroadcast{}")
	}

	eventLink := broadcast.GetEvent()
	targets := broadcast.GetTargets()

	// Early exit if no targets
	if len(targets) == 0 {
		return nil
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id":      eventLink.GetRoomId(),
		"target_count": len(targets),
	})

	// Create outbox entries for each subscriber
	eventLinkData, err := dlrEH.eventRepo.GetByID(ctx, eventLink.GetEventId())
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such chat event exists")
			return nil
		}
		logger.WithError(err).Error("failed to get chat event data")
		return err
	}

	deliveryTopic, err := dlrEH.getTopic()
	if err != nil {
		logger.WithError(err).Error("failed to get topic")
		return err
	}

	// Pre-compute payload once for all targets
	payloadStruct := eventLinkData.Content.ToProtoStruct()

	// Publish all deliveries - continue on individual failures
	var failCount int
	for _, target := range targets {
		eventDelivery := &eventsv1.EventDelivery{
			Event:        eventLink,
			Target:       target,
			Payload:      payloadStruct,
			IsCompressed: false,
			RetryCount:   0,
		}

		if pubErr := deliveryTopic.Publish(ctx, eventDelivery); pubErr != nil {
			failCount++
			logger.WithError(pubErr).WithField("recipient_id", target.GetRecepientId()).
				Warn("failed to publish delivery for target")
		}
	}

	if failCount > 0 {
		logger.WithField("fail_count", failCount).Warn("some deliveries failed")
	}

	return nil
}
