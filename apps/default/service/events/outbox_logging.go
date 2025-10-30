package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const RoomOutboxLoggingEventName = "room.outbox.logging.event"

type RoomOutboxLoggingQueue struct {
	svc              *frame.Service
	subscriptionRepo repository.RoomSubscriptionRepository
	outboxRepo       repository.RoomOutboxRepository
}

func NewRoomOutboxLoggingQueue(
	ctx context.Context,
	svc *frame.Service,
	dbPool pool.Pool,
	workMan workerpool.Manager,
) *RoomOutboxLoggingQueue {

	return &RoomOutboxLoggingQueue{
		svc:              svc,
		subscriptionRepo: repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan),
		outboxRepo:       repository.NewRoomOutboxRepository(ctx, dbPool, workMan),
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
	EventLink, ok := payload.(*eventsv1.EventLink)
	if !ok {
		return errors.New("invalid payload type, expected map[string]string{}")
	}

	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": EventLink.GetRoomId(),
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging")

	// Create outbox entries for each subscriber
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, EventLink.GetRoomId(), true) // active only
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such subscribers exists")
			return nil
		}
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	outboxEntries := make([]*models.RoomOutbox, 0, len(subscriptions))
	var EventReceipts []*eventsv1.EventReceipt

	for _, sub := range subscriptions {
		outbox := &models.RoomOutbox{
			RoomID:         EventLink.GetRoomId(),
			EventID:        EventLink.GetEventId(),
			SubscriptionID: sub.GetID(),
			State:          models.RoomOutboxStateLogged,
			RetryCount:     0,
			ErrorMessage:   "",
		}
		outbox.GenID(ctx)
		outboxEntries = append(outboxEntries, outbox)
		EventReceipts = append(EventReceipts, &eventsv1.EventReceipt{
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
			Event:    EventLink,
			Targets:  EventReceipts,
			Priority: 0,
		}

		err = csq.svc.Emit(ctx, RoomOutboxDeliveryEventName, &eventBroadcast)
		if err != nil {
			logger.WithError(err).Error(" failed to publish event broadcast")
			return nil
		}
	}

	logger.Debug("Successfully created outbox entries")
	return nil
}
