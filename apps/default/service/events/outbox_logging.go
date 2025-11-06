package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const RoomOutboxLoggingEventName = "room.outbox.logging.event"

type RoomOutboxLoggingQueue struct {
	evtsManager      frevents.Manager
	subscriptionRepo repository.RoomSubscriptionRepository
	outboxRepo       repository.RoomOutboxRepository
}

func NewRoomOutboxLoggingQueue(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	evtsManager frevents.Manager,
) *RoomOutboxLoggingQueue {
	return &RoomOutboxLoggingQueue{
		subscriptionRepo: repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan),
		outboxRepo:       repository.NewRoomOutboxRepository(ctx, dbPool, workMan),
		evtsManager:      evtsManager,
	}
}

func (csq *RoomOutboxLoggingQueue) Name() string {
	return RoomOutboxLoggingEventName
}

func (csq *RoomOutboxLoggingQueue) PayloadType() any {
	return &eventsv1.EventLink{}
}

func (csq *RoomOutboxLoggingQueue) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.EventLink)
	if !ok {
		return errors.New("invalid payload type, expected *string")
	}

	return nil
}

func (csq *RoomOutboxLoggingQueue) Execute(ctx context.Context, payload any) error {
	evtLink, ok := payload.(*eventsv1.EventLink)
	if !ok {
		return errors.New("invalid payload type, expected map[string]string{}")
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": evtLink.GetRoomId(),
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging")

	// Create outbox entries for each subscriber
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, evtLink.GetRoomId(), true) // active only
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such subscribers exists")
			return nil
		}
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	outboxEntries := make([]*models.RoomOutbox, 0, len(subscriptions))
	var evtReceipts []*eventsv1.EventReceipt

	for _, sub := range subscriptions {
		if sub.ProfileID == evtLink.GetSenderId() {
			continue
		}

		outbox := &models.RoomOutbox{
			RoomID:         evtLink.GetRoomId(),
			EventID:        evtLink.GetEventId(),
			SubscriptionID: sub.GetID(),
			OutboxState:    models.RoomOutboxStateLogged,
			RetryCount:     0,
			ErrorMessage:   "",
		}
		outbox.GenID(ctx)
		outboxEntries = append(outboxEntries, outbox)
		evtReceipts = append(evtReceipts, &eventsv1.EventReceipt{
			RecepientId: sub.ProfileID,
			TargetId:    outbox.GetID(),
		})
	}

	// Save outbox entries and update unread counts
	if len(outboxEntries) > 0 {
		err = csq.outboxRepo.BulkCreate(ctx, outboxEntries)
		if err != nil {
			logger.WithError(err).Error("failed to create new outbox users")
			return err
		}

		eventBroadcast := eventsv1.EventBroadcast{
			Event:    evtLink,
			Targets:  evtReceipts,
			Priority: 0,
		}

		err = csq.evtsManager.Emit(ctx, RoomOutboxDeliveryEventName, &eventBroadcast)
		if err != nil {
			logger.WithError(err).Error(" failed to publish event broadcast")
			return nil
		}
	}

	logger.Debug("Successfully created outbox entries")
	return nil
}
