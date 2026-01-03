package business

import (
	"context"
	"fmt"
	"time"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type connectBusiness struct {
	service         *frame.Service
	subRepo         repository.RoomSubscriptionRepository
	eventRepo       repository.RoomEventRepository
	subscriptionSvc SubscriptionService

	presenceCache cache.Cache[string, *chatv1.PresenceEvent]

	deliveryTopic queue.Publisher
}

// NewConnectBusiness creates a new instance of ClientStateBusiness.
func NewConnectBusiness(
	service *frame.Service,
	subRepo repository.RoomSubscriptionRepository,
	eventRepo repository.RoomEventRepository,
	subscriptionSvc SubscriptionService,
) ClientStateBusiness {
	return &connectBusiness{
		service:         service,
		subRepo:         subRepo,
		eventRepo:       eventRepo,
		subscriptionSvc: subscriptionSvc,
	}
}

// UpdatePresence sends presence updates to all related profiles.
func (cb *connectBusiness) UpdatePresence(
	ctx context.Context,
	presenceEvt *chatv1.PresenceEvent,
) error {
	source := presenceEvt.GetSource()
	if source == nil || source.GetProfileId() == "" {
		return service.ErrUnspecifiedID
	}

	return cb.presenceCache.Set(ctx, source.GetProfileId(), presenceEvt, 1*time.Minute)
}

// UpdateTypingIndicator sends typing indicators to room subscribers.
func (cb *connectBusiness) UpdateTypingIndicator(
	ctx context.Context,
	profileID string,
	roomID string,
	isTyping bool,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	hasAccess, err := cb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}
	if !hasAccess {
		return service.ErrRoomAccessDenied
	}

	// Broadcast user is typing to other room members
	// Note: STATE_TYPING events don't have typed payload content
	typingEvent := &eventsv1.Delivery{
		Event: &eventsv1.Link{
			EventId: util.IDString(),
			RoomId:  roomID,
			Source: &commonv1.ContactLink{
				ProfileId: profileID,
			},
			EventType: chatv1.RoomEventType_ROOM_EVENT_TYPE_TYPING,
			CreatedAt: timestamppb.Now(),
		},
		IsCompressed: false,
		RetryCount:   0,
	}

	return cb.broadCast(ctx, roomID, typingEvent)
}

// UpdateReadReceipt update read receipt and notifies room subscribers.
func (cb *connectBusiness) UpdateReadReceipt(
	ctx context.Context,
	profileID string,
	roomID string,
	eventID string,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	hasAccess, err := cb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}
	if !hasAccess {
		return service.ErrRoomAccessDenied
	}

	// Update the subscription's last read event ID
	sub, err := cb.subRepo.GetOneByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update to the new event ID
	// UnreadCount is now a generated column and will be automatically calculated
	if sub.LastReadEventID < eventID {
		sub.LastReadEventID = eventID
		sub.LastReadAt = time.Now().Unix()

		_, err = cb.subRepo.Update(ctx, sub)
		if err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}
	}

	// Broadcast read receipt to other room members
	receiptEvent := &eventsv1.Delivery{
		Event: &eventsv1.Link{
			EventId: eventID,
			RoomId:  roomID,
			Source: &commonv1.ContactLink{
				ProfileId: profileID,
			},
			EventType: chatv1.RoomEventType_ROOM_EVENT_TYPE_READ,
			CreatedAt: timestamppb.Now(),
		},
		IsCompressed: false,
		RetryCount:   0,
	}

	return cb.broadCast(ctx, roomID, receiptEvent)
}

func (cb *connectBusiness) UpdateReadMarker(
	ctx context.Context,
	profileID string,
	roomID string,
	upToEventID string,
) error {
	if profileID == "" {
		return service.ErrUnspecifiedID
	}
	if roomID == "" {
		return service.ErrRoomIDRequired
	}

	// Check if user has access to the room
	hasAccess, err := cb.subscriptionSvc.HasAccess(ctx, profileID, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}
	if !hasAccess {
		return service.ErrRoomAccessDenied
	}

	// Update the subscription's last read event ID
	sub, err := cb.subRepo.GetOneByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	subLastReadEventID := sub.LastReadEventID
	// Update to the new event ID
	// UnreadCount is now a generated column and will be automatically calculated
	if subLastReadEventID < upToEventID {
		subLastReadEventID = upToEventID
	}

	if subLastReadEventID != sub.LastReadEventID {
		sub.LastReadAt = time.Now().Unix()
		if _, err = cb.subRepo.Update(ctx, sub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}
	}

	var receiptEvents []*eventsv1.Delivery

	// Broadcast read receipt to other room members
	receiptEvents = append(receiptEvents, &eventsv1.Delivery{
		Event: &eventsv1.Link{
			EventId: upToEventID,
			RoomId:  roomID,
			Source: &commonv1.ContactLink{
				ProfileId: profileID,
			},
			EventType: chatv1.RoomEventType_ROOM_EVENT_TYPE_SYSTEM,
			CreatedAt: timestamppb.Now(),
		},
		IsCompressed: false,
		RetryCount:   0,
	})

	return cb.broadCast(ctx, roomID, receiptEvents...)
}

func (cb *connectBusiness) broadCast(ctx context.Context, roomID string, dlrPayloads ...*eventsv1.Delivery) error {
	// Get the subscriptions tied to the room
	subs, err := cb.subRepo.GetByRoomID(ctx, roomID, true)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	for _, sub := range subs {
		for _, pl := range dlrPayloads {
			pl.Destination = &commonv1.ContactLink{
				ProfileId: sub.ProfileID,
			}

			err = cb.deliveryTopic.Publish(ctx, pl)
			if err != nil {
				util.Log(ctx).WithError(err).Error("failed to deliver receipt to subscriptions")
				return err
			}
		}
	}

	return nil
}
