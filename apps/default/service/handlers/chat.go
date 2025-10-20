package handlers

import (
	"context"

	"connectrpc.com/connect"
	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/apis/go/chat/v1/chatv1connect"
	notificationv1 "github.com/antinvestor/apis/go/notification/v1"
	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Constants for pagination and batch sizes.
const (
	// MaxBatchSize defines the maximum number of items to process in a single batch.
	MaxBatchSize = 50
)

type ChatServer struct {
	Service         *frame.Service
	NotificationCli *notificationv1.NotificationClient
	ProfileCli      *profilev1.ProfileClient
	ConnectBusiness business.ConnectBusiness
	RoomBusiness    business.RoomBusiness
	MessageBusiness business.MessageBusiness

	chatv1connect.UnimplementedChatServiceHandler
}

// NewChatServer creates a new ChatServer with the business layers initialized.
func NewChatServer(
	ctx context.Context,
	service *frame.Service,
	notificationCli *notificationv1.NotificationClient,
	profileCli *profilev1.ProfileClient,
) *ChatServer {
	roomRepo := repository.NewRoomRepository(service)
	eventRepo := repository.NewRoomEventRepository(service)
	subRepo := repository.NewRoomSubscriptionRepository(service)
	outboxRepo := repository.NewRoomOutboxRepository(service)

	// Initialize business layers
	subscriptionSvc := business.NewSubscriptionService(service, subRepo)
	messageBusiness := business.NewMessageBusiness(service, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	connectBusiness := business.NewConnectBusiness(service, subRepo, eventRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(service, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return &ChatServer{
		Service:         service,
		NotificationCli: notificationCli,
		ConnectBusiness: connectBusiness,
		RoomBusiness:    roomBusiness,
		MessageBusiness: messageBusiness,
	}
}

func (ps *ChatServer) toAPIError(err error) error {
	grpcError, ok := status.FromError(err)

	if ok {
		return grpcError.Err()
	}

	if frame.ErrorIsNoRows(err) {
		return status.Error(codes.NotFound, err.Error())
	}

	return grpcError.Err()
}

// These methods are for profile service - not needed for chat service
// Removed GetById and ListRelationships

func (ps *ChatServer) Connect(
	ctx context.Context,
	stream *connect.BidiStream[chatv1.ConnectRequest, chatv1.ServerEvent],
) error {
	// Extract profile ID from context metadata
	authClaims := frame.ClaimsFromContext(ctx)
	if authClaims == nil {
		return connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	// Extract device ID from context (optional)
	deviceID := authClaims.GetDeviceID()
	if deviceID == "" {
		deviceID = "default"
	}

	// Create stream wrapper
	streamWrapper := &bidiStreamWrapper{stream: stream}

	// Handle the connection
	err := ps.ConnectBusiness.HandleConnection(ctx, profileID, deviceID, streamWrapper)
	if err != nil {
		return err
	}

	return nil
}

func (ps *ChatServer) SendEvent(
	ctx context.Context,
	req *connect.Request[chatv1.SendEventRequest],
) (*connect.Response[chatv1.SendEventResponse], error) {
	authClaims := frame.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	// Send the message
	acks, err := ps.MessageBusiness.SendEvents(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.SendEventResponse{
		Ack: acks,
	}), nil
}

func (ps *ChatServer) GetHistory(
	ctx context.Context,
	req *connect.Request[chatv1.GetHistoryRequest],
) (*connect.Response[chatv1.GetHistoryResponse], error) {
	authClaims := frame.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	events, err := ps.MessageBusiness.GetHistory(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	// Convert to ServerEvent format
	serverEvents := make([]*chatv1.ServerEvent, 0, len(events))
	for _, event := range events {
		serverEvents = append(serverEvents, &chatv1.ServerEvent{
			Payload: &chatv1.ServerEvent_Message{
				Message: event,
			},
		})
	}

	return connect.NewResponse(&chatv1.GetHistoryResponse{
		Events: serverEvents,
	}), nil
}

func (ps *ChatServer) CreateRoom(
	ctx context.Context,
	req *connect.Request[chatv1.CreateRoomRequest],
) (*connect.Response[chatv1.CreateRoomResponse], error) {
	authClaims := frame.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	room, err := ps.RoomBusiness.CreateRoom(ctx, req.Msg, profileID)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
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
	authClaims := frame.ClaimsFromContext(ctx)
	if authClaims == nil {
		return nil, connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()

	subscriptions, err := ps.RoomBusiness.SearchRoomSubscriptions(ctx, req.Msg, profileID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&chatv1.SearchRoomSubscriptionsResponse{
		Members: subscriptions,
	}), nil
}

// Helper methods

// bidiStreamWrapper wraps connect.BidiStream to implement business.ConnectionStream.
type bidiStreamWrapper struct {
	stream *connect.BidiStream[chatv1.ConnectRequest, chatv1.ServerEvent]
}

func (w *bidiStreamWrapper) Receive() (*chatv1.ConnectRequest, error) {
	return w.stream.Receive()
}

func (w *bidiStreamWrapper) Send(event *chatv1.ServerEvent) error {
	return w.stream.Send(event)
}
