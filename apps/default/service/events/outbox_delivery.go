package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/queue"
)

const RoomOutboxDeliveryEventName = "outbox.delivery.event"

type OutboxDeliveryEventHandler struct {
	service       *frame.Service
	eventRepo     repository.RoomEventRepository
	deliveryTopic queue.Publisher
}

func NewOutboxDeliveryEventHandler(service *frame.Service) *OutboxDeliveryEventHandler {

	workMan := service.WorkManager()
	dbPool := service.DatastoreManager().GetPool(context.Background(), datastore.DefaultPoolName)

	return &OutboxDeliveryEventHandler{
		service:   service,
		eventRepo: repository.NewRoomEventRepository(dbPool, workMan),
	}
}

func (csq *OutboxDeliveryEventHandler) Name() string {
	return RoomOutboxDeliveryEventName
}

func (csq *OutboxDeliveryEventHandler) PayloadType() any {
	return &eventsv1.EventBroadcast{}
}

func (csq *OutboxDeliveryEventHandler) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.EventBroadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.EventBroadcast")
	}
	return nil
}

func (csq *OutboxDeliveryEventHandler) Execute(ctx context.Context, payload any) error {
	broadcast, ok := payload.(*eventsv1.EventBroadcast)
	if !ok {
		return errors.New("invalid payload type, expected eventsv1.EventBroadcast{}")
	}

	// Lazy init publisher if not set
	if csq.deliveryTopic == nil {
		pub, err := csq.service.GetPublisher("user.event.delivery")
		if err != nil {
			return errors.New("failed to get delivery publisher: " + err.Error())
		}
		csq.deliveryTopic = pub
	}

	chatEvent := broadcast.Event

	logger := csq.service.Log(ctx).WithFields(map[string]any{
		"room_id": chatEvent.GetRoomId(),
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox delivery map")

	// Create outbox entries for each subscriber
	chatEventData, err := csq.eventRepo.GetByID(ctx, chatEvent.GetEventId())
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such chat event exists")
			return nil
		}
		logger.WithError(err).Error("failed to get chat event data")
		return err
	}

	for _, target := range broadcast.Targets {
		userDelivery := &eventsv1.UserDelivery{
			Event:        chatEvent,
			Target:       target,
			Payload:      chatEventData.Content.ToProtoStruct(),
			IsCompressed: false,
			RetryCount:   0,
		}

		err = csq.deliveryTopic.Publish(ctx, userDelivery)
		if err != nil {
			logger.WithError(err).Error("failed to deliver event to user")
			return err
		}
	}

	logger.Debug("Successfully created queued message to user")
	return nil
}
