package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"buf.build/gen/go/antinvestor/notification/connectrpc/go/notification/v1/notificationv1connect"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service"
	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
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
	Service            *frame.Service
	NotificationCli    notificationv1connect.NotificationServiceClient
	ProfileCli         profilev1connect.ProfileServiceClient
	ConnectBusiness    business.ClientStateBusiness
	RoomBusiness       business.RoomBusiness
	MessageBusiness    business.MessageBusiness
	ProposalManagement business.ProposalManagement

	chatv1connect.UnimplementedChatServiceHandler
}

// NewChatServer creates a new ChatServer with the business layers initialized.
func NewChatServer(
	ctx context.Context,
	service *frame.Service,
	notificationCli notificationv1connect.NotificationServiceClient,
	profileCli profilev1connect.ProfileServiceClient,
	authzMiddleware authz.Middleware,
) *ChatServer {
	workMan := service.WorkManager()
	evtsMan := service.EventsManager()
	dbPool := service.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
	proposalRepo := repository.NewProposalRepository(ctx, dbPool, workMan)

	// Initialize business layers
	subscriptionSvc := business.NewSubscriptionService(service, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	connectBusiness := business.NewConnectBusiness(evtsMan, subRepo, eventRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(
		service,
		roomRepo,
		eventRepo,
		subRepo,
		proposalRepo,
		subscriptionSvc,
		messageBusiness,
		profileCli,
		authzMiddleware,
	)
	proposalManagement := business.NewRoomProposalManagement(proposalRepo, roomBusiness, subscriptionSvc)

	return &ChatServer{
		Service:            service,
		NotificationCli:    notificationCli,
		ProfileCli:         profileCli,
		ConnectBusiness:    connectBusiness,
		RoomBusiness:       roomBusiness,
		MessageBusiness:    messageBusiness,
		ProposalManagement: proposalManagement,
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
	if err != nil {
		return nil, err
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
	acks, err := ps.MessageBusiness.SendEvents(timeoutCtx, req.Msg, authenticatedContact)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"auth":        authenticatedContact,
			"event_count": len(req.Msg.GetEvent()),
		}).Error("Failed to send events")
		return nil, ps.toAPIError(ctx, err)
	}

	util.Log(ctx).WithFields(map[string]any{
		"auth":        authenticatedContact,
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
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
	const defaultLimit = int32(50) // Default limit
	limit := defaultLimit
	if req.Msg.GetCursor() != nil && req.Msg.GetCursor().GetLimit() > 0 {
		limit = req.Msg.GetCursor().GetLimit()
		if limit > MaxBatchSize {
			limit = MaxBatchSize // Cap at max batch size
		}
	}

	// Create timeout context for the operation
	timeoutCtx, cancel := ps.withTimeout(ctx, updateStateTimeout)
	defer cancel()

	events, err := ps.MessageBusiness.GetHistory(timeoutCtx, req.Msg, authenticatedContact)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"auth":    authenticatedContact,
			"room_id": req.Msg.GetRoomId(),
			"limit":   limit,
		}).Error("Failed to get history")
		return nil, ps.toAPIError(ctx, err)
	}

	// Convert to ServerEvent format with pre-allocated slice for efficiency
	util.Log(ctx).WithFields(map[string]any{
		"auth":        authenticatedContact,
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	room, err := ps.RoomBusiness.CreateRoom(ctx, req.Msg, authenticatedContact)
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
	if err != nil {
		return err
	}

	// Input validation
	if req.Msg == nil {
		return connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("request cannot be nil"),
		)
	}

	rooms, err := ps.RoomBusiness.SearchRooms(ctx, req.Msg, authenticatedContact)
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	room, err := ps.RoomBusiness.UpdateRoom(ctx, req.Msg, authenticatedContact)
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
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	err = ps.RoomBusiness.DeleteRoom(ctx, req.Msg, authenticatedContact)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.DeleteRoomResponse{}), nil
}

//nolint:dupl // structurally similar to RemoveRoomSubscriptions but differs in types and business method
func (ps *ChatServer) AddRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.AddRoomSubscriptionsRequest],
) (*connect.Response[chatv1.AddRoomSubscriptionsResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	err = ps.RoomBusiness.AddRoomSubscriptions(ctx, req.Msg, authenticatedContact)
	if err != nil {
		// Check for partial batch error - some members added, some failed
		if pbe, ok := service.IsPartialBatchError(err); ok {
			return connect.NewResponse(&chatv1.AddRoomSubscriptionsResponse{
				RoomId: req.Msg.GetRoomId(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInvalidArgument),
					Message: pbe.Error(),
				},
			}), nil
		}
		return nil, err
	}

	return connect.NewResponse(&chatv1.AddRoomSubscriptionsResponse{
		RoomId: req.Msg.GetRoomId(),
	}), nil
}

//nolint:dupl // structurally similar to AddRoomSubscriptions but differs in types and business method
func (ps *ChatServer) RemoveRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.RemoveRoomSubscriptionsRequest],
) (*connect.Response[chatv1.RemoveRoomSubscriptionsResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	err = ps.RoomBusiness.RemoveRoomSubscriptions(ctx, req.Msg, authenticatedContact)
	if err != nil {
		// Check for partial batch error - some removals succeeded, some failed lookup
		if pbe, ok := service.IsPartialBatchError(err); ok {
			return connect.NewResponse(&chatv1.RemoveRoomSubscriptionsResponse{
				RoomId: req.Msg.GetRoomId(),
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeInvalidArgument),
					Message: pbe.Error(),
				},
			}), nil
		}
		return nil, err
	}

	return connect.NewResponse(&chatv1.RemoveRoomSubscriptionsResponse{
		RoomId: req.Msg.GetRoomId(),
	}), nil
}

func (ps *ChatServer) UpdateSubscriptionRole(
	ctx context.Context,
	req *connect.Request[chatv1.UpdateSubscriptionRoleRequest],
) (*connect.Response[chatv1.UpdateSubscriptionRoleResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	err = ps.RoomBusiness.UpdateSubscriptionRole(ctx, req.Msg, authenticatedContact)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.UpdateSubscriptionRoleResponse{}), nil
}

func (ps *ChatServer) SearchRoomSubscriptions(
	ctx context.Context,
	req *connect.Request[chatv1.SearchRoomSubscriptionsRequest],
) (*connect.Response[chatv1.SearchRoomSubscriptionsResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	subscriptions, err := ps.RoomBusiness.SearchRoomSubscriptions(ctx, req.Msg, authenticatedContact)
	if err != nil {
		return nil, ps.toAPIError(ctx, err)
	}

	return connect.NewResponse(&chatv1.SearchRoomSubscriptionsResponse{
		Members: subscriptions,
	}), nil
}

// ListProposals returns proposals for a room, optionally filtered by state.
func (ps *ChatServer) ListProposals(
	ctx context.Context,
	req *connect.Request[chatv1.ListProposalsRequest],
) (*connect.Response[chatv1.ListProposalsResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	// Get pending proposals (business layer handles access checks)
	proposals, err := ps.ProposalManagement.ListPending(ctx, req.Msg.GetRoomId(), authenticatedContact)
	if err != nil {
		return nil, ps.toAPIError(ctx, err)
	}

	// Convert models to API types
	apiProposals := make([]*chatv1.Proposal, 0, len(proposals))
	for _, p := range proposals {
		apiProposals = append(apiProposals, proposalToAPI(p))
	}

	return connect.NewResponse(&chatv1.ListProposalsResponse{
		Proposals: apiProposals,
	}), nil
}

// SubmitProposal handles approval or rejection of a proposal.
func (ps *ChatServer) SubmitProposal(
	ctx context.Context,
	req *connect.Request[chatv1.SubmitProposalRequest],
) (*connect.Response[chatv1.SubmitProposalResponse], error) {
	authenticatedContact, err := internal.AuthContactLink(ctx)
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

	if req.Msg.GetProposalId() == "" {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("proposal_id must be specified"),
		)
	}

	// Process based on action
	switch req.Msg.GetAction() {
	case chatv1.ProposalAction_PROPOSAL_ACTION_APPROVE:
		err = ps.ProposalManagement.Approve(
			ctx,
			req.Msg.GetRoomId(),
			req.Msg.GetProposalId(),
			authenticatedContact,
		)
	case chatv1.ProposalAction_PROPOSAL_ACTION_REJECT:
		err = ps.ProposalManagement.Reject(
			ctx,
			req.Msg.GetRoomId(),
			req.Msg.GetProposalId(),
			req.Msg.GetReason(),
			authenticatedContact,
		)
	case chatv1.ProposalAction_PROPOSAL_ACTION_UNSPECIFIED:
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("action must be APPROVE or REJECT"),
		)
	}

	if err != nil {
		// Return error in response for expected errors
		if isProposalError(err) {
			return connect.NewResponse(&chatv1.SubmitProposalResponse{
				Error: &commonv1.ErrorDetail{
					Code:    int32(connect.CodeOf(err)), //nolint:gosec // connect codes fit in int32
					Message: err.Error(),
				},
			}), nil
		}
		return nil, ps.toAPIError(ctx, err)
	}

	return connect.NewResponse(&chatv1.SubmitProposalResponse{}), nil
}

// Live handles client realtime updates (typing indicators, presence, read receipts).
func (ps *ChatServer) Live(
	ctx context.Context,
	req *connect.Request[chatv1.LiveRequest],
) (*connect.Response[chatv1.LiveResponse], error) {
	// Authentication check
	authenticatedContact, err := internal.AuthContactLink(ctx, "system_internal")
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
		if processErr := ps.processClientState(ctx, clientState, authenticatedContact); processErr != nil {
			errorList = append(errorList, fmt.Sprintf("client state at index %d: %v", i, processErr))
			continue
		}
	}

	// Prepare response with aggregated error details
	response := &chatv1.LiveResponse{}

	if len(errorList) > 0 {
		total := len(req.Msg.GetClientStates())
		failed := len(errorList)
		succeeded := total - failed

		response.Error = &commonv1.ErrorDetail{
			Code: int32(connect.CodeInvalidArgument),
			Message: fmt.Sprintf(
				"partial batch failure: %d/%d client states failed: %s",
				failed, total, strings.Join(errorList, "; ")),
		}

		// If all failed, return error via gRPC status
		if succeeded == 0 {
			return nil, connect.NewError(
				connect.CodeInvalidArgument,
				fmt.Errorf("all %d client states failed: %s", failed, strings.Join(errorList, "; ")),
			)
		}
	}

	return connect.NewResponse(response), nil
}

// Helper methods

// processClientState handles individual client state processing with comprehensive validation.
func (ps *ChatServer) processClientState(
	ctx context.Context,
	clientCommand *chatv1.ClientCommand,
	authenticatedContact *commonv1.ContactLink,
) error {
	// Process different state types
	switch state := clientCommand.GetState().(type) {
	case *chatv1.ClientCommand_ReadMarker:
		return ps.processReadMarkerState(ctx, state.ReadMarker, authenticatedContact)
	case *chatv1.ClientCommand_Event:
		return ps.processRoomEventState(ctx, state.Event, authenticatedContact)
	case *chatv1.ClientCommand_Receipt:
		return ps.processDeliveryReceiptState(ctx, state.Receipt, authenticatedContact)
	case *chatv1.ClientCommand_Typing:
		return ps.processTypingState(ctx, state.Typing, authenticatedContact)
	case *chatv1.ClientCommand_Presence:
		return ps.processPresenceState(ctx, state.Presence, authenticatedContact)
	default:
		util.Log(ctx).WithFields(map[string]any{
			"auth":       authenticatedContact,
			"state_type": fmt.Sprintf("%T", state),
		}).Warn("Unknown client command type received")
		return fmt.Errorf("unsupported client command type: %T", state)
	}
}

// processDeliveryReceiptState handles receipt acknowledgments.
func (ps *ChatServer) processDeliveryReceiptState(
	ctx context.Context,
	receipt *chatv1.ReceiptEvent,
	authenticatedContact *commonv1.ContactLink,
) error {
	if receipt == nil {
		return errors.New("receipt event cannot be nil")
	}

	// Validate event_ids are provided
	if len(receipt.GetEventId()) == 0 {
		return errors.New("at least one event_id must be provided")
	}

	// Process delivery receipts for each event

	err := ps.ConnectBusiness.UpdateDeliveryReceipt(
		ctx,
		receipt.GetRoomId(),
		authenticatedContact,
		receipt.GetEventId()...)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"room_id":  receipt.GetRoomId(),
			"event_id": receipt.GetEventId(),
		}).Error("Failed to send delivery receipt")
		return fmt.Errorf("failed to update delivery receipts for events: %w", err)
	}

	return nil
}

// processReadMarkerState handles read marker updates.
func (ps *ChatServer) processReadMarkerState(
	ctx context.Context,
	readMarker *chatv1.ReadMarker,
	authenticatedContact *commonv1.ContactLink,
) error {
	if readMarker == nil {
		return errors.New("read marker event cannot be nil")
	}

	if readMarker.GetUpToEventId() == "" {
		return errors.New("up_to_event_id must be specified")
	}

	// Mark messages as read up to the specified event
	if err := ps.ConnectBusiness.UpdateReadMarker(
		ctx,
		readMarker.GetRoomId(),
		authenticatedContact,
		readMarker.GetUpToEventId(),
	); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"auth":           authenticatedContact,
			"room_id":        readMarker.GetRoomId(),
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
	authenticatedContact *commonv1.ContactLink,
) error {
	if typing == nil {
		return errors.New("typing event cannot be nil")
	}

	if typing.GetRoomId() == "" {
		return errors.New("room_id must be specified")
	}

	// Set timestamp if not provided
	if typing.GetSince() == nil {
		typing.Since = timestamppb.Now()
	}

	// Send typing indicator
	if err := ps.ConnectBusiness.UpdateTypingIndicator(
		ctx,
		typing.GetRoomId(),
		authenticatedContact,
		typing.GetTyping(),
	); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"auth":    authenticatedContact,
			"room_id": typing.GetRoomId(),
			"typing":  typing.GetTyping(),
		}).Error("Failed to send typing indicator")
		return fmt.Errorf("failed to send typing indicator: %w", err)
	}

	return nil
}

// processPresenceState handles presence updates.
func (ps *ChatServer) processPresenceState(
	ctx context.Context,
	presence *chatv1.PresenceEvent,
	authenticatedContact *commonv1.ContactLink,
) error {
	if presence == nil {
		return errors.New("presence event cannot be nil")
	}

	// Set timestamp if not provided
	if presence.GetLastActive() == nil {
		presence.LastActive = timestamppb.Now()
	}

	// Create presence event
	presenceEvent := &chatv1.PresenceEvent{
		Source:     authenticatedContact,
		Status:     presence.GetStatus(),
		StatusMsg:  presence.GetStatusMsg(),
		LastActive: timestamppb.Now(),
	}

	// Send presence update
	if err := ps.ConnectBusiness.UpdatePresence(ctx, presenceEvent); err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"auth":   authenticatedContact,
			"status": presence.GetStatus(),
		}).Error("Failed to send presence update")
		return fmt.Errorf("failed to send presence update: %w", err)
	}

	return nil
}

// processRoomEventState handles room events (messages, etc.)
func (ps *ChatServer) processRoomEventState(
	ctx context.Context,
	roomEvent *chatv1.RoomEvent,
	sender *commonv1.ContactLink,
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

	_, err := ps.MessageBusiness.SendEvents(ctx, sendReq, sender)
	if err != nil {
		util.Log(ctx).WithError(err).WithFields(map[string]any{
			"sender":     sender,
			"room_id":    roomEvent.GetRoomId(),
			"event_type": roomEvent.GetType(),
		}).Error("Failed to send room event")
		return fmt.Errorf("failed to send room event: %w", err)
	}

	return nil
}

// proposalToAPI converts a models.Proposal to a chatv1.Proposal.
func proposalToAPI(p *models.Proposal) *chatv1.Proposal {
	proposal := &chatv1.Proposal{
		Id:          p.ID,
		RoomId:      p.ScopeID,
		Type:        proposalTypeToAPI(p.ProposalType),
		State:       proposalStateToAPI(p.State),
		RequestedBy: p.RequestedBy,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		ExpiresAt:   timestamppb.New(p.ExpiresAt),
	}

	// Convert payload map to structpb
	if p.Payload != nil {
		if payload, err := structpb.NewStruct(p.Payload); err == nil {
			proposal.Payload = payload
		}
	}

	// Set optional fields
	if p.ResolvedBy != "" {
		proposal.ResolvedBy = &p.ResolvedBy
	}
	if p.ResolvedAt != nil {
		proposal.ResolvedAt = timestamppb.New(*p.ResolvedAt)
	}
	if p.Reason != "" {
		proposal.Reason = &p.Reason
	}

	return proposal
}

// proposalTypeToAPI converts models.ProposalType to chatv1.ProposalType.
func proposalTypeToAPI(t models.ProposalType) chatv1.ProposalType {
	switch t {
	case models.ProposalTypeUpdateRoom:
		return chatv1.ProposalType_PROPOSAL_TYPE_UPDATE_ROOM
	case models.ProposalTypeDeleteRoom:
		return chatv1.ProposalType_PROPOSAL_TYPE_DELETE_ROOM
	case models.ProposalTypeAddSubscriptions:
		return chatv1.ProposalType_PROPOSAL_TYPE_ADD_SUBSCRIPTIONS
	case models.ProposalTypeRemoveSubscriptions:
		return chatv1.ProposalType_PROPOSAL_TYPE_REMOVE_SUBSCRIPTIONS
	case models.ProposalTypeUpdateSubscriptionRole:
		return chatv1.ProposalType_PROPOSAL_TYPE_UPDATE_SUBSCRIPTION_ROLE
	default:
		return chatv1.ProposalType_PROPOSAL_TYPE_UNSPECIFIED
	}
}

// proposalStateToAPI converts models.ProposalState to chatv1.ProposalState.
func proposalStateToAPI(s models.ProposalState) chatv1.ProposalState {
	switch s {
	case models.ProposalStatePending:
		return chatv1.ProposalState_PROPOSAL_STATE_PENDING
	case models.ProposalStateApproved:
		return chatv1.ProposalState_PROPOSAL_STATE_APPROVED
	case models.ProposalStateRejected:
		return chatv1.ProposalState_PROPOSAL_STATE_REJECTED
	case models.ProposalStateExpired:
		return chatv1.ProposalState_PROPOSAL_STATE_EXPIRED
	default:
		return chatv1.ProposalState_PROPOSAL_STATE_UNSPECIFIED
	}
}

// isProposalError returns true if the error is a known proposal error.
func isProposalError(err error) bool {
	return errors.Is(err, service.ErrProposalNotFound) ||
		errors.Is(err, service.ErrProposalNotPending) ||
		errors.Is(err, service.ErrProposalExpired) ||
		errors.Is(err, service.ErrProposalApprovalDenied)
}
