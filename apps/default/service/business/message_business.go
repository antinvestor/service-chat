package business

import (
	"context"
	"fmt"
	"time"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	eventsv1 "github.com/antinvestor/service-chat/proto/events/v1"
	"github.com/pitabwire/frame/data"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type messageBusiness struct {
	eventRepo       repository.RoomEventRepository
	outboxRepo      repository.RoomOutboxRepository
	subRepo         repository.RoomSubscriptionRepository
	subscriptionSvc SubscriptionService
	evtsManager     frevents.Manager
}

// NewMessageBusiness creates a new instance of MessageBusiness.
func NewMessageBusiness(
	evtsManager frevents.Manager,
	eventRepo repository.RoomEventRepository,
	outboxRepo repository.RoomOutboxRepository,
	subRepo repository.RoomSubscriptionRepository,
	subscriptionSvc SubscriptionService,
) MessageBusiness {
	return &messageBusiness{

		evtsManager:     evtsManager,
		eventRepo:       eventRepo,
		outboxRepo:      outboxRepo,
		subRepo:         subRepo,
		subscriptionSvc: subscriptionSvc,
	}
}

//nolint:gocognit,funlen // Complex event validation and processing logic required for message delivery
func (mb *messageBusiness) SendEvents(
	ctx context.Context,
	req *chatv1.SendEventRequest,
	senderID string,
) ([]*chatv1.StreamAck, error) {
	// Validate request
	if len(req.GetEvent()) == 0 {
		return nil, service.ErrMessageContentRequired
	}

	requestEvents := req.GetEvent()

	// Pre-allocate response slice to maintain request order
	responses := make([]*chatv1.StreamAck, len(requestEvents))
	validEvents := make([]*models.RoomEvent, 0, len(requestEvents))
	eventToIndex := make(map[string]int, len(requestEvents))

	// Extract unique room IDs for batch access checking
	uniqueRoomIDs := make(map[string]bool)
	for _, reqEvt := range requestEvents {
		if reqEvt.GetRoomId() == "" {
			return nil, service.ErrMessageRoomIDRequired
		}
		uniqueRoomIDs[reqEvt.GetRoomId()] = true
	}

	// Batch check access for all unique rooms
	roomAccessMap := make(map[string]bool, len(uniqueRoomIDs))
	roomAccessErrors := make(map[string]error, len(uniqueRoomIDs))

	for roomID := range uniqueRoomIDs {
		hasAccess, accessErr := mb.subscriptionSvc.HasAccess(ctx, senderID, roomID)
		if accessErr != nil {
			roomAccessErrors[roomID] = accessErr
		} else {
			roomAccessMap[roomID] = hasAccess
		}
	}

	// Collect all event IDs for deduplication check
	eventIDsToCheck := make([]string, 0, len(requestEvents))
	for _, reqEvt := range requestEvents {
		if reqEvt.GetId() != "" {
			eventIDsToCheck = append(eventIDsToCheck, reqEvt.GetId())
		}
	}

	// Check for existing events (idempotency/deduplication)
	existingEvents := make(map[string]bool)
	if len(eventIDsToCheck) > 0 {
		var err error
		existingEvents, err = mb.eventRepo.ExistsByIDs(ctx, eventIDsToCheck)
		if err != nil {
			util.Log(ctx).WithError(err).Warn("Failed to check for existing events, proceeding without deduplication")
			// Continue without deduplication rather than failing the entire request
		}
	}

	// Phase 1: Validate all events and prepare valid ones for bulk save
	for i, reqEvt := range requestEvents {
		// Assign ID if not provided
		if reqEvt.GetId() == "" {
			reqEvt.Id = util.IDString()
		}

		// Map event ID to its position in the request for ordered responses
		eventToIndex[reqEvt.GetId()] = i

		roomID := reqEvt.GetRoomId()

		// Check for duplicate event (idempotency)
		if existingEvents[reqEvt.GetId()] {
			// Event already exists - return success (idempotent response)
			success, _ := structpb.NewStruct(map[string]any{
				"status":     "ok",
				"idempotent": true,
			})
			responses[i] = &chatv1.StreamAck{
				EventId:  reqEvt.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: success,
			}
			continue
		}

		// Check if there was an error checking access for this room
		if accessErr, hasError := roomAccessErrors[roomID]; hasError {
			errorD, _ := structpb.NewStruct(
				map[string]any{"error": fmt.Sprintf("failed to check room access: %v", accessErr)},
			)
			responses[i] = &chatv1.StreamAck{
				EventId:  reqEvt.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			}
			continue
		}

		// Check if user has access to this room
		hasAccess := roomAccessMap[roomID]
		if !hasAccess {
			errorD, _ := structpb.NewStruct(map[string]any{"error": service.ErrMessageSendDenied.Error()})
			responses[i] = &chatv1.StreamAck{
				EventId:  reqEvt.GetId(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			}
			continue
		}

		// Create the message event
		event := &models.RoomEvent{
			RoomID:    reqEvt.GetRoomId(),
			SenderID:  senderID,
			ParentID:  reqEvt.GetParentId(),
			EventType: int32(reqEvt.GetType().Number()),
			Content:   data.JSONMap(reqEvt.GetPayload().AsMap()),
		}

		event.GenID(ctx)
		if reqEvt.GetId() != "" {
			event.ID = reqEvt.GetId()
		}

		validEvents = append(validEvents, event)
	}

	// Phase 2: Bulk save all valid events
	if len(validEvents) == 0 {
		return responses, nil
	}

	bulkCreateErr := mb.eventRepo.BulkCreate(ctx, validEvents)

	// Phase 3: Process each valid event - emit to outbox or report errors
	for _, event := range validEvents {
		responseIdx := eventToIndex[event.GetID()]

		// Check if bulk save failed
		if bulkCreateErr != nil {
			errorD, _ := structpb.NewStruct(
				map[string]any{"error": fmt.Sprintf("failed to save event: %v", bulkCreateErr)},
			)
			responses[responseIdx] = &chatv1.StreamAck{
				EventId:  event.GetID(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			}
			continue
		}

		// Emit event to outbox for delivery
		outboxEventLink := eventsv1.EventLink{
			EventId:   event.GetID(),
			RoomId:    event.RoomID,
			SenderId:  &event.SenderID,
			ParentId:  event.ParentID,
			EventType: eventsv1.EventLink_EventType(event.EventType),
			CreatedAt: timestamppb.New(event.CreatedAt),
		}

		emitErr := mb.evtsManager.Emit(ctx, events.RoomOutboxLoggingEventName, &outboxEventLink)
		if emitErr != nil {
			errorD, _ := structpb.NewStruct(map[string]any{"error": fmt.Sprintf("failed to emit event: %v", emitErr)})
			responses[responseIdx] = &chatv1.StreamAck{
				EventId:  event.GetID(),
				AckAt:    timestamppb.Now(),
				Metadata: errorD,
			}
			continue
		}

		// Success - event saved and emitted to outbox
		success, _ := structpb.NewStruct(map[string]any{"status": "ok"})
		responses[responseIdx] = &chatv1.StreamAck{
			EventId:  event.GetID(),
			AckAt:    timestamppb.Now(),
			Metadata: success,
		}
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

	err = mb.eventRepo.Delete(ctx, event.GetID())
	if err != nil {
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
	sub, err := mb.subRepo.GetOneByRoomAndProfile(ctx, roomID, profileID)
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

	if _, updateErr := mb.subRepo.Update(ctx, sub); updateErr != nil {
		return fmt.Errorf("failed to update subscription: %w", updateErr)
	}

	return nil
}
