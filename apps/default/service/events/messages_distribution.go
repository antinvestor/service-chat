package events

import (
	"context"
	"errors"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame/datastore/pool"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
)

const (
	RoomOutboxLoggingEventName = "room.outbox.logging.event"
	defaultBatchSize           = 1000
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
	return &eventsv1.Link{}
}

func (csq *RoomOutboxLoggingQueue) Validate(_ context.Context, payload any) error {
	_, ok := payload.(*eventsv1.Link)
	if !ok {
		return errors.New("invalid payload type, expected *RoomOutboxPayload or *eventsv1.Link")
	}
	return nil
}

func (csq *RoomOutboxLoggingQueue) Execute(ctx context.Context, payload any) error {
	// Unwrap payload
	evtLink, ok := payload.(*eventsv1.Link)
	if !ok {
		return errors.New("invalid payload type")
	}

	roomID := evtLink.GetRoomId()
	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": roomID,
		"cursor":  evtLink.GetCursor(),
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging batch")

	// Fetch one batch of subscribers
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, roomID, evtLink.GetCursor())
	if err != nil {
		logger.WithError(err).Error("failed to get room subscribers")
		return err
	}

	if len(subscriptions) == 0 {
		logger.Debug("no more subscribers to process")
		return nil
	}

	var source *commonv1.ContactLink
	var destinations []*commonv1.ContactLink
	for _, sub := range subscriptions {
		if sub.GetID() == evtLink.GetSourceSubscriptionId() {
			source = sub.ToLink()
			continue
		}

		// Only broadcast messages to active subscriptions
		if sub.IsActive() {
			destinations = append(destinations, sub.ToLink())
		}
	}

	// Emit broadcast for this batch
	if len(destinations) > 0 {
		eventBroadcast := eventsv1.Broadcast{
			Event:        evtLink,
			Source:       source,
			Destinations: destinations,
			Priority:     0,
		}
		if err = csq.evtsManager.Emit(ctx, RoomFanoutEventName, &eventBroadcast); err != nil {
			logger.WithError(err).Error("failed to publish event broadcast")
			return err
		}
		logger.WithField("batch_size", len(destinations)).Debug("emitted broadcast batch")
	}

	// If we fetched a full batch, there might be more subscribers. Emit a new job with the next cursor.
	if len(subscriptions) >= defaultBatchSize {
		nextCursor := subscriptions[len(subscriptions)-1].GetID()
		evtLink.SetCursor(&commonv1.PageCursor{
			Limit: defaultBatchSize,
			Page:  nextCursor,
		})
		if err = csq.evtsManager.Emit(ctx, RoomOutboxLoggingEventName, evtLink); err != nil {
			logger.WithError(err).Error("failed to emit next batch job")
			return err
		}
		logger.WithField("next_cursor", nextCursor).Debug("emitted next batch job")
	}

	return nil
}
