package business

import (
	"context"
	"fmt"
	"time"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type messageBusiness struct {
	service         *frame.Service
	eventRepo       repository.RoomEventRepository
	outboxRepo      repository.RoomOutboxRepository
	subRepo         repository.RoomSubscriptionRepository
	subscriptionSvc SubscriptionService
}

// NewMessageBusiness creates a new instance of MessageBusiness.
func NewMessageBusiness(
	service *frame.Service,
	eventRepo repository.RoomEventRepository,
	outboxRepo repository.RoomOutboxRepository,
	subRepo repository.RoomSubscriptionRepository,
	subscriptionSvc SubscriptionService,
) MessageBusiness {
	return &messageBusiness{
		service:         service,
		eventRepo:       eventRepo,
		outboxRepo:      outboxRepo,
		subRepo:         subRepo,
		subscriptionSvc: subscriptionSvc,
	}
}

func (mb *messageBusiness) SendEvents(
	ctx context.Context,
	req *chatv1.SendEventRequest,
	senderID string,
) ([]*chatv1.StreamAck, error) {
	// Validate request
	if len(req.GetEvent()) == 0 {
		return nil, service.ErrMessageContentRequired
	}

	var responses []*chatv1.StreamAck

	for _, msg := range req.GetEvent() {
		if msg.GetRoomId() == "" {
			return nil, service.ErrMessageRoomIDRequired
		}
		// Check if the sender has access to the room
		hasAccess, err := mb.subscriptionSvc.HasAccess(ctx, senderID, msg.GetRoomId())
		if err != nil {
			errorD, _ := structpb.NewStruct(
				map[string]any{"error": fmt.Sprintf("failed to check room access: %v", err)},
			)
			responses = append(responses, &chatv1.StreamAck{
				EventId:  msg.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			})

			continue
		}

		if !hasAccess {
			errorD, _ := structpb.NewStruct(map[string]any{"error": service.ErrMessageSendDenied.Error()})
			responses = append(responses, &chatv1.StreamAck{
				EventId:  msg.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			})
			continue
		}

		// Create the message event
		event := &models.RoomEvent{
			RoomID:      msg.GetRoomId(),
			MessageType: msg.GetType().String(),
			SenderID:    senderID,
			Content:     data.JSONMap(msg.GetPayload().AsMap()), // Content is in payload
		}

		event.GenID(ctx)
		if msg.GetId() != "" {
			event.ID = msg.GetId()
		}

		// Start a transaction to ensure atomicity
		// Save the event
		if err = mb.eventRepo.Save(ctx, event); err != nil {
			errorD, _ := structpb.NewStruct(map[string]any{"error": fmt.Sprintf("failed to save event: %v", err)})
			responses = append(responses, &chatv1.StreamAck{
				EventId:  msg.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			})

			continue
		}

		outboxIDMap := map[string]string{
			"room_id":       msg.GetRoomId(),
			"room_event_id": event.GetID(),
			"sender_id":     senderID,
		}

		err = mb.service.Emit(ctx, events.RoomOutboxLoggingEventName, outboxIDMap)
		if err != nil {
			errorD, _ := structpb.NewStruct(map[string]any{"error": fmt.Sprintf("failed to emit event: %v", err)})
			responses = append(responses, &chatv1.StreamAck{
				EventId:  msg.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			})

			continue
		}

		// Unread counts are now automatically calculated as a generated column
		// based on pending outbox entries
		success, _ := structpb.NewStruct(map[string]any{"success": "ok"})
		responses = append(responses, &chatv1.StreamAck{
			EventId:  event.GetID(),
			AckAt:    timestamppb.Now(),
			Metadata: success,
		})
	}

	return responses, nil
}

func (mb *messageBusiness) GetMessage(
	ctx context.Context,
	messageID string,
	profileID string,
) (*models.RoomEvent, error) {
	if messageID == "" {
		return nil, service.ErrUnspecifiedID
	}

	// Get the message
	event, err := mb.eventRepo.GetByID(ctx, messageID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return nil, service.ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	// Check if the user has access to the room
	hasAccess, err := mb.subscriptionSvc.HasAccess(ctx, profileID, event.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	if !hasAccess {
		return nil, service.ErrMessageAccessDenied
	}

	return event, nil
}

func (mb *messageBusiness) GetHistory(
	ctx context.Context,
	req *chatv1.GetHistoryRequest,
	profileID string,
) ([]*chatv1.RoomEvent, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrMessageRoomIDRequired
	}

	// Check if the user has access to the room
	hasAccess, err := mb.subscriptionSvc.HasAccess(ctx, profileID, req.GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	if !hasAccess {
		return nil, service.ErrRoomAccessDenied
	}

	// Build the query - use cursor for pagination
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 50 // default limit
	}

	// Get messages
	evts, err := mb.eventRepo.GetHistory(ctx, req.GetRoomId(), "", "", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get message history: %w", err)
	}

	// Convert to proto
	protoEvents := make([]*chatv1.RoomEvent, 0, len(evts))
	for _, event := range evts {
		protoEvents = append(protoEvents, event.ToAPI())
	}

	// Update last read sequence for the user if we have evts
	if len(evts) > 0 && req.GetCursor() == "" {
		_ = mb.MarkMessagesAsRead(ctx, req.GetRoomId(), evts[0].GetID(), profileID)
	}

	return protoEvents, nil
}

func (mb *messageBusiness) DeleteMessage(ctx context.Context, messageID string, profileID string) error {
	if messageID == "" {
		return service.ErrUnspecifiedID
	}

	// Get the message
	event, err := mb.eventRepo.GetByID(ctx, messageID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return service.ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message: %w", err)
	}

	// Check if the user is the sender or an admin
	isSender := event.SenderID == profileID
	isAdmin, err := mb.subscriptionSvc.HasRole(ctx, profileID, event.RoomID, "admin")
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isSender && !isAdmin {
		return service.ErrMessageDeleteDenied
	}

	// Soft delete the message
	event.DeletedAt = time.Now().Unix()
	if err := mb.eventRepo.Save(ctx, event); err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	return nil
}

func (mb *messageBusiness) MarkMessagesAsRead(
	ctx context.Context,
	roomID string,
	eventID string,
	profileID string,
) error {
	if roomID == "" {
		return service.ErrMessageRoomIDRequired
	}
	if profileID == "" {
		return service.ErrUnspecifiedID
	}

	// Get the subscription
	sub, err := mb.subRepo.GetByRoomAndProfile(ctx, roomID, profileID)
	if err != nil {
		if data.ErrorIsNoRows(err) {
			return service.ErrSubscriptionNotFound
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Update the last read event ID
	// UnreadCount is now a generated column and will be automatically calculated
	sub.LastReadEventID = eventID
	sub.LastReadAt = time.Now().Unix()

	if err := mb.subRepo.Save(ctx, sub); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}
