package handlers

import (
	"context"

	"connectrpc.com/connect"
	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/apis/go/chat/v1/chatv1connect"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GatewayServer handles the gateway-specific chat service operations.
// It focuses on the Connect functionality for real-time communication with edge devices.
type GatewayServer struct {
	Service           *frame.Service
	ChatServiceClient chatv1connect.ChatServiceClient
	ConnectionManager *business.ConnectionManager

	chatv1connect.UnimplementedGatewayServiceHandler
}

// NewGatewayServer creates a new gateway server instance.
func NewGatewayServer(
	service *frame.Service,
	chatServiceClient chatv1connect.ChatServiceClient,
	connectionManager *business.ConnectionManager,
) *GatewayServer {
	return &GatewayServer{
		Service:           service,
		ChatServiceClient: chatServiceClient,
		ConnectionManager: connectionManager,
	}
}

// Connect handles bidirectional streaming connections from edge devices.
// This is the primary method for maintaining real-time connections.
func (gs *GatewayServer) Connect(
	ctx context.Context,
	stream *connect.BidiStream[chatv1.ConnectRequest, chatv1.ServerEvent],
) error {
	// Extract profile ID from context
	authClaims := security.ClaimsFromContext(ctx)
	if authClaims == nil {
		return connect.NewError(
			connect.CodeUnauthenticated,
			status.Error(codes.Unauthenticated, "request needs to be authenticated"),
		)
	}

	profileID, _ := authClaims.GetSubject()
	deviceID := authClaims.GetDeviceID()
	if deviceID == "" {
		deviceID = "default"
	}

	gs.Service.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"device_id":  deviceID,
	}).Info("New device connection request")

	// Create stream wrapper
	streamWrapper := &deviceStreamWrapper{stream: stream}

	// Handle the connection through the connection manager
	err := gs.ConnectionManager.HandleConnection(ctx, profileID, deviceID, streamWrapper)
	if err != nil {
		gs.Service.Log(ctx).WithError(err).Error("Connection ended with error")
		return err
	}

	return nil
}

// deviceStreamWrapper wraps connect.BidiStream to implement business.DeviceStream.
type deviceStreamWrapper struct {
	stream *connect.BidiStream[chatv1.ConnectRequest, chatv1.ServerEvent]
}

func (w *deviceStreamWrapper) Receive() (*chatv1.ConnectRequest, error) {
	return w.stream.Receive()
}

func (w *deviceStreamWrapper) Send(event *chatv1.ServerEvent) error {
	return w.stream.Send(event)
}
