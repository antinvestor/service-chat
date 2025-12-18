package events

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/models"
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

	// roomSubscriberCacheTTL is the time-to-live for cached room subscribers.
	// Set to 30 seconds to balance freshness with query reduction.
	roomSubscriberCacheTTL = 30 * time.Second
)

// roomSubscriberCacheEntry holds cached room subscriber data.
type roomSubscriberCacheEntry struct {
	subscribers []*models.RoomSubscription
	expiresAt   time.Time
}

// roomSubscriberCache provides TTL-based caching for room subscribers.
// This reduces database queries for high-traffic rooms.
type roomSubscriberCache struct {
	mu    sync.RWMutex
	cache map[string]*roomSubscriberCacheEntry
}

func newRoomSubscriberCache() *roomSubscriberCache {
	return &roomSubscriberCache{
		cache: make(map[string]*roomSubscriberCacheEntry),
	}
}

func (c *roomSubscriberCache) get(roomID string) ([]*models.RoomSubscription, bool) {
	c.mu.RLock()
	entry, exists := c.cache[roomID]
	c.mu.RUnlock()

	if !exists || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.subscribers, true
}

func (c *roomSubscriberCache) set(roomID string, subscribers []*models.RoomSubscription) {
	c.mu.Lock()
	c.cache[roomID] = &roomSubscriberCacheEntry{
		subscribers: subscribers,
		expiresAt:   time.Now().Add(roomSubscriberCacheTTL),
	}
	c.mu.Unlock()
}

// invalidate removes a room from cache (call when membership changes).
func (c *roomSubscriberCache) invalidate(roomID string) {
	c.mu.Lock()
	delete(c.cache, roomID)
	c.mu.Unlock()
}

type RoomOutboxLoggingQueue struct {
	evtsManager       frevents.Manager
	subscriptionRepo  repository.RoomSubscriptionRepository
	outboxRepo        repository.RoomOutboxRepository
	subscriberCache   *roomSubscriberCache
}

func NewRoomOutboxLoggingQueue(
	ctx context.Context,
	dbPool pool.Pool,
	workMan workerpool.Manager,
	evtsManager frevents.Manager,
) *RoomOutboxLoggingQueue {
	return &RoomOutboxLoggingQueue{
		subscriptionRepo:  repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan),
		outboxRepo:        repository.NewRoomOutboxRepository(ctx, dbPool, workMan),
		evtsManager:       evtsManager,
		subscriberCache:   newRoomSubscriberCache(),
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

	// Try to get subscribers from cache first
	subscriptions, cacheHit := csq.subscriberCache.get(roomID)
	if !cacheHit {
		// Cache miss - fetch from database
		var err error
		subscriptions, err = csq.subscriptionRepo.GetByRoomID(ctx, roomID, true) // active only
		if err != nil {
			if data.ErrorIsNoRows(err) {
				logger.WithError(err).Error("no such subscribers exists")
				return nil
			}
			logger.WithError(err).Error("failed to get room subscribers")
			return err
		}
		// Cache the result for subsequent messages in this room
		csq.subscriberCache.set(roomID, subscriptions)
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
		if err := csq.outboxRepo.BulkCreate(ctx, outboxEntries); err != nil {
			logger.WithError(err).Error("failed to create new outbox users")
			return err
		}

		eventBroadcast := eventsv1.EventBroadcast{
			Event:    evtLink,
			Targets:  evtReceipts,
			Priority: 0,
		}

		if err := csq.evtsManager.Emit(ctx, RoomOutboxDeliveryEventName, &eventBroadcast); err != nil {
			logger.WithError(err).Error(" failed to publish event broadcast")
			return nil
		}
	}

	logger.Debug("Successfully created outbox entries")
	return nil
}
