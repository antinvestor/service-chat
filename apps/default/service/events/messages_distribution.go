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

// RoomOutboxPayload wraps eventsv1.Link with pagination cursor for recursive processing.
type RoomOutboxPayload struct {
	Link   *eventsv1.Link
	Cursor *commonv1.PageCursor
}

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
	return &RoomOutboxPayload{}
}

func (csq *RoomOutboxLoggingQueue) Validate(_ context.Context, payload any) error {
	switch p := payload.(type) {
	case *RoomOutboxPayload:
		if p.Link == nil {
			return errors.New("invalid payload: Link is nil")
		}
		return nil
	case *eventsv1.Link:
		// Accept raw Link for backwards compatibility (initial event)
		return nil
	default:
		return errors.New("invalid payload type, expected *RoomOutboxPayload or *eventsv1.Link")
	}
}

func (csq *RoomOutboxLoggingQueue) Execute(ctx context.Context, payload any) error {
	var evtLink *eventsv1.Link
	var cursor *commonv1.PageCursor

	// Unwrap payload
	switch p := payload.(type) {
	case *RoomOutboxPayload:
		evtLink = p.Link
		cursor = p.Cursor
	case *eventsv1.Link:
		evtLink = p
		cursor = nil
	default:
		return errors.New("invalid payload type")
	}

	roomID := evtLink.GetRoomId()
	logger := util.Log(ctx).WithFields(map[string]any{
		"room_id": roomID,
		"cursor":  cursor,
		"type":    csq.Name(),
	})
	logger.Debug("handling outbox logging batch")

	// Fetch one batch of subscribers
	subscriptions, err := csq.subscriptionRepo.GetByRoomID(ctx, roomID, cursor)
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
		nextPayload := &RoomOutboxPayload{
			Link: evtLink,
			Cursor: &commonv1.PageCursor{
				Limit: defaultBatchSize,
				Page:  nextCursor,
			},
		}
		if err = csq.evtsManager.Emit(ctx, RoomOutboxLoggingEventName, nextPayload); err != nil {
			logger.WithError(err).Error("failed to emit next batch job")
			return err
		}
		logger.WithField("next_cursor", nextCursor).Debug("emitted next batch job")
	}

	return nil
}
