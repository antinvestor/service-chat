package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
)

const RoomOutboxLoggingEventName = "room.outbox.logging.event"

type RoomOutboxLoggingQueue struct {
	Service          *frame.Service
	subscriptionRepo repository.RoomSubscriptionRepository
	outboxRepo       repository.RoomOutboxRepository
}

func NewRoomOutboxLoggingQueue(service *frame.Service) *RoomOutboxLoggingQueue {

	workMan := service.WorkManager()
	dbPool := service.DatastoreManager().GetPool(context.Background(), datastore.DefaultPoolName)

	return &RoomOutboxLoggingQueue{
		Service:          service,
		subscriptionRepo: repository.NewRoomSubscriptionRepository(dbPool, workMan),
		outboxRepo:       repository.NewRoomOutboxRepository(dbPool, workMan),
	}
}

func (csq *RoomOutboxLoggingQueue) Name() string {
	return RoomOutboxLoggingEventName
}

func (csq *RoomOutboxLoggingQueue) PayloadType() any {
	return &eventsv1.ChatEvent{}
}

func (csq *RoomOutboxLoggingQueue) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.ChatEvent)
	if !ok {
		return errors.New("invalid payload type, expected *string")
	}

	return nil
}

func (csq *RoomOutboxLoggingQueue) Execute(ctx context.Context, payload any) error {
	chatEvent, ok := payload.(*eventsv1.ChatEvent)
	if !ok {
		return errors.New("invalid payload type, expected map[string]string{}")
	}

	logger := csq.Service.Log(ctx).WithFields(map[string]any{
		"room_id": chatEvent.GetRoomId(),
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging")

	// Create outbox entries for each subscriber
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, chatEvent.GetRoomId(), true) // active only
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such subscribers exists")
			return nil
		}
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	outboxEntries := make([]*models.RoomOutbox, 0, len(subscriptions))
	var deliveryTargets []*eventsv1.DeliveryTarget

	for _, sub := range subscriptions {
		outbox := &models.RoomOutbox{
			RoomID:         chatEvent.GetRoomId(),
			EventID:        chatEvent.GetEventId(),
			SubscriptionID: sub.GetID(),
			Status:         "pending",
			RetryCount:     0,
			ErrorMessage:   "",
		}
		outbox.GenID(ctx)
		outboxEntries = append(outboxEntries, outbox)
		deliveryTargets = append(deliveryTargets, &eventsv1.DeliveryTarget{
			RecepientId: sub.ProfileID,
			OutboxId:    outbox.GetID(),
		})
	}

	// Save outbox entries and update unread counts
	if len(outboxEntries) > 0 {
		err = csq.outboxRepo.BatchInsert(ctx, outboxEntries)
		if err != nil {
			logger.WithError(err).Error("failed to create new outbox users")
			return err
		}

		eventBroadcast := eventsv1.EventBroadcast{
			Event:    chatEvent,
			Targets:  deliveryTargets,
			Priority: 0,
		}

		err = csq.Service.Emit(ctx, RoomOutboxDeliveryEventName, &eventBroadcast)
		if err != nil {
			logger.WithError(err).Error(" failed to publish event broadcast")
			return nil
		}
	}

	logger.Debug("Successfully created outbox entries")
	return nil
}
