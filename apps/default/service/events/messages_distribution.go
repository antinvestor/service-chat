package events

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore/pool"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const (
	RoomOutboxLoggingEventName = "room.outbox.logging.event"
)

type RoomOutboxLoggingQueue struct {
	evtsManager      frevents.Manager
	subscriptionRepo repository.RoomSubscriptionRepository
}

func NewRoomOutboxLoggingQueue(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	evtsManager frevents.Manager,
) *RoomOutboxLoggingQueue {
	return &RoomOutboxLoggingQueue{
		subscriptionRepo: repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan),
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

	roomID := evtLink.GetRoomId()
	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": roomID,
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging")

	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, roomID, true) // active only
	if err != nil {
		if data.ErrorIsNoRows(err) {
			logger.WithError(err).Error("no such subscribers exists")
			return nil
		}
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	var recepients []string

	for _, sub := range subscriptions {
		if sub.ProfileID == evtLink.GetSenderId() {
			continue
		}

		recepients = append(recepients, sub.ProfileID)
	}

	// Save outbox entries and update unread counts
	if len(recepients) == 0 {
		logger.Debug("no recepients exist to receive messages")
		return nil
	}
	eventBroadcast := eventsv1.EventBroadcast{
		Event:        evtLink,
		RecepientIds: recepients,
		Priority:     0,
	}

	if err = csq.evtsManager.Emit(ctx, RoomFanoutEventName, &eventBroadcast); err != nil {
		logger.WithError(err).Error(" failed to publish event broadcast")
		return nil
	}

	logger.Debug("successful message distribution")
	return nil
}
