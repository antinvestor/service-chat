package events

import (
	"context"
	"errors"

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
	eventRepo     repository.RoomEventRepository
	deliveryTopic queue.Publisher
}

func NewOutboxDeliveryEventHandler(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	deliveryTopic queue.Publisher,
) *OutboxDeliveryEventHandler {
	return &OutboxDeliveryEventHandler{
		deliveryTopic: deliveryTopic,
		eventRepo:     repository.NewRoomEventRepository(ctx, dbPool, workMan),
	}
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

	EventLink := broadcast.GetEvent()

	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": EventLink.GetRoomId(),
		"type":    dlrEH.Name(),
	})
	logger.Debug("handling outbox delivery map")

	// Create outbox entries for each subscriber
	EventLinkData, err := dlrEH.eventRepo.GetByID(ctx, EventLink.GetEventId())
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such chat event exists")
			return nil
		}
		logger.WithError(err).Error("failed to get chat event data")
		return err
	}

	for _, target := range broadcast.GetTargets() {
		EventDelivery := &eventsv1.EventDelivery{
			Event:        EventLink,
			Target:       target,
			Payload:      EventLinkData.Content.ToProtoStruct(),
			IsCompressed: false,
			RetryCount:   0,
		}

		err = dlrEH.deliveryTopic.Publish(ctx, EventDelivery)
		if err != nil {
			logger.WithError(err).Error("failed to deliver event to user")
			return err
		}
	}

	logger.Debug("Successfully created queued message to user")
	return nil
}
