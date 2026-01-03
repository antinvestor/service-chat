package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"
	"buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"buf.build/gen/go/antinvestor/notification/connectrpc/go/notification/v1/notificationv1connect"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Constants for pagination and batch sizes.
const (
	// MaxBatchSize defines the maximum number of items to process in a single batch.
	MaxBatchSize = 50

	// Default timeout constants.
	defaultTimeout     = 30 * time.Second
	sendEventsTimeout  = 10 * time.Second
	updateStateTimeout = 15 * time.Second
)

type ChatServer struct {
	Service         *frame.Service
	NotificationCli notificationv1connect.NotificationServiceClient
	ProfileCli      profilev1connect.ProfileServiceClient
	ConnectBusiness business.ClientStateBusiness
	RoomBusiness    business.RoomBusiness
	MessageBusiness business.MessageBusiness

	chatv1connect.UnimplementedChatServiceHandler
}

// NewChatServer creates a new ChatServer with the business layers initialized.
func NewChatServer(
	ctx context.Context,
	service *frame.Service,
	notificationCli notificationv1connect.NotificationServiceClient,
	profileCli profilev1connect.ProfileServiceClient) *ChatServer {
	workMan := service.WorkManager()
	evtsMan := service.EventsManager()
	dbPool := service.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	// Initialize business layers
	subscriptionSvc := business.NewSubscriptionService(service, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	connectBusiness := business.NewConnectBusiness(service, subRepo, eventRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(service, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness, profileCli)

	return &ChatServer{
		Service:         service,
		NotificationCli: notificationCli,
		ProfileCli:      profileCli,
		ConnectBusiness: connectBusiness,
		RoomBusiness:    roomBusiness,
		MessageBusiness: messageBusiness,
	}
}

// toAPIError converts internal errors to appropriate gRPC status codes.
func (ps *ChatServer) toAPIError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	// Check if it's already a gRPC status error
	if grpcError, ok := status.FromError(err); ok {
		return grpcError.Err()
	}

	// Handle specific error types
	switch {
	case data.ErrorIsNoRows(err):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		// Log internal errors for debugging
		util.Log(ctx).WithError(err).Error("Internal server error")
		return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
	}
}

// validateAuthentication extracts and validates authentication claims.
func (ps *ChatServer) validateAuthentication(ctx context.Context) (string, error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return "", connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("request needs to be authenticated"),
		)
	}

	profileID, err := authClaims.GetSubject()
	if err != nil || profileID == "" {
		return "", connect.NewError(
			connect.CodeUnauthenticated,
			errors.New("invalid authentication claims"),
		)
	}

	return profileID, nil
}

// withTimeout creates a context with timeout for resource efficiency.
func (ps *ChatServer) withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (ps *ChatServer) SendEvent(
	ctx context.Context,
	req *connect.Request[chatv1.SendEventRequest],
) (*connect.Response[chatv1.SendEventResponse], error) {
	// Validate authentication
	profileID, err := ps.validateAuthentication(ctx)
	if err != nil {
		return nil, err
	}

	// Input validation
	if req.Msg == nil {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("request cannot be nil"),
		)
	}

	if len(req.Msg.GetEvent()) == 0 {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("at least one event must be provided"),
		)
	}

	// Limit batch size for resource efficiency
	if len(req.Msg.GetEvent()) > MaxBatchSize {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("too many events: max %d allowed", MaxBatchSize),
		)
	}

	// Create timeout context for the operation
	timeoutCtx, cancel := ps.withTimeout(ctx, sendEventsTimeout)
	defer cancel()

	// Send the events
	acks, err := ps.MessageBusiness.SendEvents(timeoutCtx, req.Msg, profileID)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id":  profileID,
			"event_count": len(req.Msg.GetEvent()),
		}).Error("Failed to send events")
		return nil, ps.toAPIError(ctx, err)
	}

	util.Log(ctx).WithFields(map[string]any{
		"profile_id":  profileID,
		"event_count": len(req.Msg.GetEvent()),
		"ack_count":   len(acks),
	}).Debug("Events sent successfully")

	return connect.NewResponse(&chatv1.SendEventResponse{
		Ack: acks,
	}), nil
}

func (ps *ChatServer) GetHistory(
	ctx context.Context,
	req *connect.Request[chatv1.GetHistoryRequest],
) (*connect.Response[chatv1.GetHistoryResponse], error) {
	// Validate authentication
	profileID, err := ps.validateAuthentication(ctx)
	if err != nil {
		return nil, err
	}

	// Input validation
	if req.Msg == nil {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("request cannot be nil"),
		)
	}

	if req.Msg.GetRoomId() == "" {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("room_id must be specified"),
		)
	}

	// Validate pagination parameters
	limit := int32(50) // Default limit
	if req.Msg.GetCursor() != nil && req.Msg.GetCursor().GetLimit() > 0 {
		limit = req.Msg.GetCursor().GetLimit()
		if limit > MaxBatchSize {
			limit = MaxBatchSize // Cap at max batch size
		}
	}

	// Create timeout context for the operation
	timeoutCtx, cancel := ps.withTimeout(ctx, updateStateTimeout)
	defer cancel()

	events, err := ps.MessageBusiness.GetHistory(timeoutCtx, req.Msg, profileID)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id": profileID,
			"room_id":    req.Msg.GetRoomId(),
			"limit":      limit,
		}).Error("Failed to get history")
		return nil, ps.toAPIError(ctx, err)
	}

	// Convert to ServerEvent format with pre-allocated slice for efficiency
	util.Log(ctx).WithFields(map[string]any{
		"profile_id":  profileID,
		"room_id":     req.Msg.GetRoomId(),
		"event_count": len(events),
		"limit":       limit,
	}).Debug("History retrieved successfully")

	return connect.NewResponse(&chatv1.GetHistoryResponse{
		Events: events,
	}), nil
}

func (ps *ChatServer) CreateRoom(
	ctx context.Context,
	req *connect.Request[chatv1.CreateRoomRequest],
) (*connect.Response[chatv1.CreateRoomResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	// Create ContactLink for the creator
	creatorLink := &commonv1.ContactLink{
		ProfileId: profileID,
		// ContactId can be set if available from profile service
	}

	room, err := ps.RoomBusiness.CreateRoom(ctx, req.Msg, creatorLink)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.CreateRoomResponse{
		Room: room,
	}), nil
}

func (ps *ChatServer) SearchRooms(
	ctx context.Context,
	req *connect.Request[chatv1.SearchRoomsRequest],
	stream *connect.ServerStream[chatv1.SearchRoomsResponse],
) error {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	rooms, err := ps.RoomBusiness.SearchRooms(ctx, req.Msg, profileID)
	if err != nil {
		return err
	}

	// Send all rooms in a single response
	return stream.Send(&chatv1.SearchRoomsResponse{
		Data: rooms,
	})
}

func (ps *ChatServer) UpdateRoom(
	ctx context.Context,
	req *connect.Request[chatv1.UpdateRoomRequest],
) (*connect.Response[chatv1.UpdateRoomResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	room, err := ps.RoomBusiness.UpdateRoom(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.UpdateRoomResponse{
		Room: room,
	}), nil
}

func (ps *ChatServer) DeleteRoom(
	ctx context.Context,
	req *connect.Request[chatv1.DeleteRoomRequest],
) (*connect.Response[chatv1.DeleteRoomResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	err := ps.RoomBusiness.DeleteRoom(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.DeleteRoomResponse{}), nil
}

func (ps *ChatServer) AddRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.AddRoomSubscriptionsRequest],
) (*connect.Response[chatv1.AddRoomSubscriptionsResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	err := ps.RoomBusiness.AddRoomSubscriptions(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.AddRoomSubscriptionsResponse{}), nil
}

func (ps *ChatServer) RemoveRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.RemoveRoomSubscriptionsRequest],
) (*connect.Response[chatv1.RemoveRoomSubscriptionsResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	err := ps.RoomBusiness.RemoveRoomSubscriptions(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.RemoveRoomSubscriptionsResponse{}), nil
}

func (ps *ChatServer) UpdateSubscriptionRole(
	ctx context.Context,
	req *connect.Request[chatv1.UpdateSubscriptionRoleRequest],
) (*connect.Response[chatv1.UpdateSubscriptionRoleResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	err := ps.RoomBusiness.UpdateSubscriptionRole(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.UpdateSubscriptionRoleResponse{}), nil
}

func (ps *ChatServer) SearchRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.SearchRoomSubscriptionsRequest],
) (*connect.Response[chatv1.SearchRoomSubscriptionsResponse], error) {
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	subscriptions, err := ps.RoomBusiness.SearchRoomSubscriptions(ctx, req.Msg, profileID)
	if err != nil {
		return nil, ps.toAPIError(ctx, err)
	}

	return connect.NewResponse(&chatv1.SearchRoomSubscriptionsResponse{
		Members: subscriptions,
	}), nil
}

// UpdateClientState handles client state updates (typing indicators, presence, read receipts).
func (ps *ChatServer) UpdateClientState(
	ctx context.Context,
	req *connect.Request[chatv1.UpdateClientStateRequest],
) (*connect.Response[chatv1.UpdateClientStateResponse], error) {
	// Authentication check
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	// Input validation
	if req.Msg == nil {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			status.Error(codes.InvalidArgument, "request cannot be nil"),
		)
	}

	// Validate client states
	if len(req.Msg.GetClientStates()) == 0 {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			status.Error(codes.InvalidArgument, "at least one client state must be provided"),
		)
	}

	// Limit batch size for resource efficiency
	if len(req.Msg.GetClientStates()) > MaxBatchSize {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			status.Error(codes.InvalidArgument, fmt.Sprintf("too many client states: max %d allowed", MaxBatchSize)),
		)
	}

	// Process each client state
	var errorList []string

	for i, clientState := range req.Msg.GetClientStates() {
		if clientState == nil {
			errorList = append(errorList, fmt.Sprintf("client state at index %d is nil", i))
			continue
		}

		// Validate and process the client state
		if err := ps.processClientState(ctx, clientState, profileID, req.Msg.GetRoomId()); err != nil {
			errorList = append(errorList, fmt.Sprintf("client state at index %d: %v", i, err))
			continue
		}
	}

	// Prepare response
	response := &chatv1.UpdateClientStateResponse{}

	// If there were errorList, return first error
	if len(errorList) > 0 {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("failed to process client states: %s", errorList[0]),
		)
	}

	return connect.NewResponse(response), nil
}

// Helper methods

// processClientState handles individual client state processing with comprehensive validation.
func (ps *ChatServer) processClientState(
	ctx context.Context,
	clientCommand *chatv1.ClientCommand,
	profileID string,
	roomID string,
) error {
	// Process different state types
	switch state := clientCommand.GetState().(type) {
	case *chatv1.ClientCommand_ReadMarker:
		return ps.processReadMarkerState(ctx, state.ReadMarker, profileID, roomID)
	case *chatv1.ClientCommand_Event:
		return ps.processRoomEventState(ctx, state.Event, profileID)
	default:
		util.Log(ctx).WithFields(map[string]any{
			"profile_id": profileID,
			"state_type": fmt.Sprintf("%T", state),
		}).Warn("Unknown client command type received")
		return fmt.Errorf("unsupported client command type: %T", state)
	}
}

// processReceiptState handles receipt acknowledgments.
func (ps *ChatServer) processReceiptState(
	ctx context.Context,
	receipt *chatv1.ReceiptEvent,
	profileID string,
	roomID string,
) error {
	if receipt == nil {
		return errors.New("receipt event cannot be nil")
	}

	// Validate and override profile_id
	receipt.SetSource(&commonv1.ContactLink{
		ProfileId: profileID,
	})

	// Use room_id from receipt if not provided in request
	targetRoomID := roomID
	if targetRoomID == "" {
		targetRoomID = receipt.GetRoomId()
	}

	if targetRoomID == "" {
		return errors.New("room_id must be specified")
	}

	// Validate event_ids are provided
	if len(receipt.GetEventId()) == 0 {
		return errors.New("at least one event_id must be provided")
	}

	// Process read receipts for each event
	for _, eventID := range receipt.GetEventId() {
		if eventID == "" {
			continue
		}

		if err := ps.ConnectBusiness.UpdateReadReceipt(ctx, profileID, targetRoomID, eventID); err != nil {
			util.Log(ctx).WithError(err).WithFields(map[string]any{
				"profile_id": profileID,
				"room_id":    targetRoomID,
				"event_id":   eventID,
			}).Error("Failed to send read receipt")
			return fmt.Errorf("failed to send read receipt for event %s: %w", eventID, err)
		}
	}

	return nil
}

// processReadMarkerState handles read marker updates.
func (ps *ChatServer) processReadMarkerState(
	ctx context.Context,
	readMarker *chatv1.ReadMarker,
	profileID string,
	roomID string,
) error {
	if readMarker == nil {
		return errors.New("read marker event cannot be nil")
	}

	// Validate and override profile_id
	readMarker.SetSource(&commonv1.ContactLink{
		ProfileId: profileID,
	})

	// Use room_id from read marker if not provided in request
	targetRoomID := roomID
	if targetRoomID == "" {
		targetRoomID = readMarker.GetRoomId()
	}

	if targetRoomID == "" {
		return errors.New("room_id must be specified")
	}

	if readMarker.GetUpToEventId() == "" {
		return errors.New("up_to_event_id must be specified")
	}

	// Mark messages as read up to the specified event
	if err := ps.ConnectBusiness.UpdateReadMarker(ctx, targetRoomID, readMarker.GetUpToEventId(), profileID); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id":     profileID,
			"room_id":        targetRoomID,
			"up_to_event_id": readMarker.GetUpToEventId(),
		}).Error("Failed to mark messages as read")
		return fmt.Errorf("failed to mark messages as read: %w", err)
	}

	return nil
}

// processTypingState handles typing indicators.
func (ps *ChatServer) processTypingState(
	ctx context.Context,
	typing *chatv1.TypingEvent,
	profileID string,
	roomID string,
) error {
	if typing == nil {
		return errors.New("typing event cannot be nil")
	}

	// Validate and override profile_id
	typing.SetSource(&commonv1.ContactLink{
		ProfileId: profileID,
	})

	// Use room_id from typing if not provided in request
	targetRoomID := roomID
	if targetRoomID == "" {
		targetRoomID = typing.GetRoomId()
	}

	if targetRoomID == "" {
		return errors.New("room_id must be specified")
	}

	// Set timestamp if not provided
	if typing.GetSince() == nil {
		typing.Since = timestamppb.Now()
	}

	// Send typing indicator
	if err := ps.ConnectBusiness.UpdateTypingIndicator(ctx, profileID, targetRoomID, typing.GetTyping()); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id": profileID,
			"room_id":    targetRoomID,
			"typing":     typing.GetTyping(),
		}).Error("Failed to send typing indicator")
		return fmt.Errorf("failed to send typing indicator: %w", err)
	}

	return nil
}

// processPresenceState handles presence updates.
func (ps *ChatServer) processPresenceState(
	ctx context.Context,
	presence *chatv1.PresenceEvent,
	profileID string,
	_ string,
) error {
	if presence == nil {
		return errors.New("presence event cannot be nil")
	}

	// Validate and override profile_id
	presence.SetSource(&commonv1.ContactLink{
		ProfileId: profileID,
	})

	// Set timestamp if not provided
	if presence.GetLastActive() == nil {
		presence.SetLastActive(timestamppb.Now())
	}

	// Create presence event
	presenceEvent := &chatv1.PresenceEvent{
		Source: &commonv1.ContactLink{
			ProfileId: profileID,
		},
		Status:     presence.GetStatus(),
		StatusMsg:  presence.GetStatusMsg(),
		LastActive: timestamppb.Now(),
	}

	// Send presence update
	if err := ps.ConnectBusiness.UpdatePresence(ctx, presenceEvent); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id": profileID,
			"status":     presence.GetStatus(),
		}).Error("Failed to send presence update")
		return fmt.Errorf("failed to send presence update: %w", err)
	}

	return nil
}

// processRoomEventState handles room events (messages, etc.)
func (ps *ChatServer) processRoomEventState(
	ctx context.Context,
	roomEvent *chatv1.RoomEvent,
	senderID string,
) error {
	if roomEvent == nil {
		return errors.New("room event cannot be nil")
	}

	// Set timestamp if not provided
	if roomEvent.GetSentAt() == nil {
		roomEvent.SentAt = timestamppb.Now()
	}

	// Send the event using the message business layer
	sendReq := &chatv1.SendEventRequest{
		Event: []*chatv1.RoomEvent{roomEvent},
	}

	_, err := ps.MessageBusiness.SendEvents(ctx, sendReq, senderID)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"profile_id": senderID,
			"room_id":    roomEvent.GetRoomId(),
			"event_type": roomEvent.GetType(),
		}).Error("Failed to send room event")
		return fmt.Errorf("failed to send room event: %w", err)
	}

	return nil
}
