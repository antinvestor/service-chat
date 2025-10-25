package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
)

const RoomOutboxLoggingQueueName = "room.outbox.logging.queue"

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
	return RoomOutboxLoggingQueueName
}

func (csq *RoomOutboxLoggingQueue) PayloadType() any {
	return &map[string]string{}
}

func (csq *RoomOutboxLoggingQueue) Validate(_ context.Context, payload any) error {
	_, ok := payload.(map[string]string)
	if !ok {
		return errors.New("invalid payload type, expected *string")
	}

	return nil
}

func (csq *RoomOutboxLoggingQueue) Execute(ctx context.Context, payload any) error {
	outboxIDMap, ok := payload.(map[string]string)
	if !ok {
		return errors.New("invalid payload type, expected map[string]string{}")
	}

	roomID := outboxIDMap["room_id"]
	roomEventID := outboxIDMap["room_event_id"]

	logger := csq.Service.Log(ctx).WithFields(map[string]any{
		"room_id": roomID,
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging")

	// Create outbox entries for each subscriber
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, roomID, true) // active only
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such subscribers exists")
			return nil
		}
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	outboxEntries := make([]*models.RoomOutbox, 0, len(subscriptions))

	for _, sub := range subscriptions {
		outbox := &models.RoomOutbox{
			RoomID:         roomID,
			EventID:        roomEventID,
			SubscriptionID: sub.GetID(),
			Status:         "pending",
			RetryCount:     0,
			ErrorMessage:   "",
		}
		outbox.GenID(ctx)
		outboxEntries = append(outboxEntries, outbox)
	}

	// Save outbox entries and update unread counts
	if len(outboxEntries) > 0 {
		err = csq.outboxRepo.BatchInsert(ctx, outboxEntries)
		if err != nil {
			logger.WithError(err).Error("failed to create new outbox users")
			return err
		}

	}

	logger.Debug("Successfully created outbox entries")
	return nil
}
