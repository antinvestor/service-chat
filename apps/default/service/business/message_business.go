package business

import (
	"context"
	"errors"
	"fmt"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame/data"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type messageBusiness struct {
	eventRepo       repository.RoomEventRepository
	subRepo         repository.RoomSubscriptionRepository
	subscriptionSvc SubscriptionService
	evtsManager     frevents.Manager

	payloadConverter *models.PayloadConverter
}

// NewMessageBusiness creates a new instance of MessageBusiness.
func NewMessageBusiness(
	evtsManager frevents.Manager,
	eventRepo repository.RoomEventRepository,
	subRepo repository.RoomSubscriptionRepository,
	subscriptionSvc SubscriptionService,
) MessageBusiness {
	return &messageBusiness{

		evtsManager:     evtsManager,
		eventRepo:       eventRepo,
		subRepo:         subRepo,
		subscriptionSvc: subscriptionSvc,

		payloadConverter: models.NewPayloadConverter(),
	}
}

//nolint:gocognit,funlen // Complex event validation and processing logic required for message delivery
func (mb *messageBusiness) SendEvents(
	ctx context.Context,
	req *chatv1.SendEventRequest,
	sentBy *commonv1.ContactLink,
) ([]*chatv1.AckEvent, error) {
	// Validate request
	if len(req.GetEvent()) == 0 {
		return nil, service.ErrMessageContentRequired
	}

	if err := internal.IsValidContactLink(sentBy); err != nil {
		return nil, err
	}

	requestEvents := req.GetEvent()

	// Pre-allocate response slice to maintain request order
	responses := make([]*chatv1.AckEvent, len(requestEvents))
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

	uniqueRoomIDList := make([]string, 0, len(uniqueRoomIDs))
	for roomID := range uniqueRoomIDs {
		uniqueRoomIDList = append(uniqueRoomIDList, roomID)
	}

	// Check access for each room individually to handle non-existent rooms
	subscriptionMap := make(map[string]*models.RoomSubscription)
	subscriptionIDAccessMap := make(map[string]bool)
	roomSubscriptionIDMap := make(map[string]string)

	for _, roomID := range uniqueRoomIDList {
		roomAccessList, accessErr := mb.subscriptionSvc.HasAccess(ctx, sentBy, roomID)
		if accessErr != nil {
			// For non-existent rooms, create empty access map
			connectError := &connect.Error{}
			if errors.As(accessErr, &connectError) {
				continue
			}
			return nil, accessErr
		}

		// Add to combined maps
		for _, sub := range roomAccessList {
			subscriptionMap[sub.GetID()] = sub
			roomSubscriptionIDMap[sub.RoomID] = sub.GetID()
			subscriptionIDAccessMap[sub.GetID()] = sub.IsActive()
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

		subscriptionID, ok := roomSubscriptionIDMap[roomID]
		if !ok {

			responses[i] = &chatv1.AckEvent{
				EventId: []string{reqEvt.GetId()},
				AckAt:   timestamppb.Now(),

				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInternal),
					Message: fmt.Sprintf("no subscription to room: %v", roomID),
				},
			}
			continue
		}

		// Check if user has access to this room
		hasAccess := subscriptionIDAccessMap[subscriptionID]
		if !hasAccess {
			responses[i] = &chatv1.AckEvent{
				EventId: []string{reqEvt.GetId()},
				AckAt:   timestamppb.Now(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodePermissionDenied),
					Message: service.ErrMessageSendDenied.Error(),
				},
			}
			continue
		}

		// Create the message event using PayloadConverter
		content, err := mb.payloadConverter.FromProto(reqEvt.GetPayload())
		if err != nil {
			responses[i] = &chatv1.AckEvent{
				EventId: []string{reqEvt.GetId()},
				AckAt:   timestamppb.Now(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInternal),
					Message: fmt.Sprintf("failed to convert event: %v", err),
				},
			}
			continue
		}

		// Create the message event
		event := &models.RoomEvent{
			RoomID:    reqEvt.GetRoomId(),
			EventType: int32(reqEvt.GetType()),
			Content:   content,
			SenderID:  subscriptionID,
		}

		if reqEvt.ParentId != nil {
			event.ParentID = reqEvt.GetParentId()
		}

		// Use client-provided ID if available, otherwise generate
		if reqEvt.GetId() != "" {
			event.ID = reqEvt.GetId()
		} else {
			event.GenID(ctx)
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
			responses[responseIdx] = &chatv1.AckEvent{
				EventId: []string{event.GetID()},
				AckAt:   timestamppb.Now(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInternal),
					Message: fmt.Sprintf("failed to save event: %v", bulkCreateErr),
				},
			}
			continue
		}

		subscription, ok := subscriptionMap[event.SenderID]
		if !ok {
			util.Log(ctx).WithField("subscription_id", event.SenderID).Error("very unlikely, no such subscription exists")
			continue
		}

		// Emit event to outbox for delivery
		outboxEventLink := eventsv1.Link{
			EventId: event.GetID(),
			RoomId:  event.RoomID,

			Source: &eventsv1.Subscription{
				SubscriptionId: subscription.GetID(),
				ContactLink:    subscription.ToLink(),
			},
			ParentId:  event.ParentID,
			EventType: chatv1.RoomEventType(event.EventType),
			CreatedAt: timestamppb.New(event.CreatedAt),
		}

		emitErr := mb.evtsManager.Emit(ctx, events.RoomOutboxLoggingEventName, &outboxEventLink)
		if emitErr != nil {
			responses[responseIdx] = &chatv1.AckEvent{
				EventId: []string{event.GetID()},
				AckAt:   timestamppb.Now(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInternal),
					Message: fmt.Sprintf("failed to emit event: %v", emitErr),
				},
			}
			continue
		}

		// Success - event saved and emitted to outbox
		responses[responseIdx] = &chatv1.AckEvent{
			EventId: []string{event.GetID()},
			AckAt:   timestamppb.Now(),
		}
	}

	return responses, nil
}

func (mb *messageBusiness) GetMessage(
	ctx context.Context,
	messageID string,
	gottenBy *commonv1.ContactLink,
) (*models.RoomEvent, error) {
	if messageID == "" {
		return nil, service.ErrUnspecifiedID
	}

	if err := internal.IsValidContactLink(gottenBy); err != nil {
		return nil, err
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
	accessList, err := mb.subscriptionSvc.HasAccess(ctx, gottenBy, event.RoomID)
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	for _, sub := range accessList {
		if sub.RoomID == event.RoomID && sub.IsActive() {
			return event, nil
		}
	}

	return nil, service.ErrMessageAccessDenied
}

func (mb *messageBusiness) GetHistory(
	ctx context.Context,
	req *chatv1.GetHistoryRequest,
	gottenBy *commonv1.ContactLink,
) ([]*chatv1.RoomEvent, error) {
	if req.GetRoomId() == "" {
		return nil, service.ErrMessageRoomIDRequired
	}

	if err := internal.IsValidContactLink(gottenBy); err != nil {
		return nil, err
	}

	// Check if the user has access to the room
	accessList, err := mb.subscriptionSvc.HasAccess(ctx, gottenBy, req.GetRoomId())
	if err != nil {
		return nil, fmt.Errorf("failed to check room access: %w", err)
	}

	for _, sub := range accessList {
		if sub.RoomID != req.GetRoomId() || !sub.IsActive() {
			return nil, service.ErrRoomAccessDenied
		}
	}

	// Build the query - use cursor for pagination
	var limit = 50 // default limit
	var cursor string
	if req.GetCursor() != nil {
		if req.GetCursor().GetLimit() > 0 {
			limit = int(req.GetCursor().GetLimit())
		}
		cursor = req.GetCursor().GetPage()
	}

	// Get messages
	evts, err := mb.eventRepo.GetHistory(ctx, req.GetRoomId(), "", "", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get message history: %w", err)
	}

	// Convert to proto
	converter := models.NewPayloadConverter()
	protoEvents := make([]*chatv1.RoomEvent, 0, len(evts))
	for _, event := range evts {
		protoEvents = append(protoEvents, event.ToAPI(ctx, converter))
	}

	// Update last read sequence for the user if we have evts
	if len(evts) > 0 && cursor == "" {
		_ = mb.MarkMessagesAsRead(ctx, req.GetRoomId(), evts[0].GetID(), gottenBy)
	}

	return protoEvents, nil
}

func (mb *messageBusiness) DeleteMessage(ctx context.Context, messageID string, deletedBy *commonv1.ContactLink) error {
	if messageID == "" {
		return service.ErrUnspecifiedID
	}

	if err := internal.IsValidContactLink(deletedBy); err != nil {
		return err
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
	isSender := event.SenderID == deletedBy.GetProfileId()
	admin, err := mb.subscriptionSvc.HasRole(ctx, deletedBy, event.RoomID, roleAdminLevel)
	if err != nil {
		return fmt.Errorf("failed to check admin status: %w", err)
	}

	if !isSender && admin != nil {
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
	markedBy *commonv1.ContactLink,
) error {
	if err := internal.IsValidContactLink(markedBy); err != nil {
		return err
	}

	if roomID == "" {
		return service.ErrMessageRoomIDRequired
	}

	// Check if the user has access to the room
	accessList, err := mb.subscriptionSvc.HasAccess(ctx, markedBy, roomID)
	if err != nil {
		return fmt.Errorf("failed to check room access: %w", err)
	}

	var subscription *models.RoomSubscription
	// Check if the user has access to the room
	for _, sub := range accessList {
		if sub.RoomID == roomID && sub.IsActive() {
			subscription = sub
		}
	}

	if subscription == nil {
		return service.ErrRoomAccessDenied
	}

	// Update the last read event ID
	// UnreadCount is now a generated column and will be automatically calculated
	subscription.LastReadEventID = eventID
	subscription.LastReadAt = time.Now().Unix()

	if _, updateErr := mb.subRepo.Update(ctx, subscription); updateErr != nil {
		return fmt.Errorf("failed to update subscription: %w", updateErr)
	}

	return nil
}
