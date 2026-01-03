package handlers

import (
	"context"
	"buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GatewayServer handles the gateway-specific chat service operations.
// It focuses on the Connect functionality for real-time communication with edge devices.
type GatewayServer struct {
	svc        *frame.Service
	chatClient chatv1connect.ChatServiceClient
	cm         business.ConnectionManager

	chatv1connect.UnimplementedGatewayServiceHandler
}

// NewGatewayServer creates a new gateway server instance.
func NewGatewayServer(
	service *frame.Service,
	chatServiceClient chatv1connect.ChatServiceClient,
	connectionManager business.ConnectionManager,
) *GatewayServer {
	return &GatewayServer{
		svc:        service,
		chatClient: chatServiceClient,
		cm:         connectionManager,
	}
}

// Stream handles bidirectional streaming connections from edge devices.
// This is the primary method for maintaining real-time connections.
func (gs *GatewayServer) Stream(
	ctx context.Context,
	stream *connect.BidiStream[chatv1.StreamRequest, chatv1.StreamResponse],
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

	gs.svc.Log(ctx).WithFields(map[string]any{
		"profile_id": profileID,
		"device_id":  deviceID,
	}).Info("New device connection request")

	// Create stream wrapper
	streamWrapper := &deviceStreamImpl{stream: stream}

	// Handle the connection through the connection manager
	err := gs.cm.HandleConnection(ctx, profileID, deviceID, streamWrapper)
	if err != nil {
		gs.svc.Log(ctx).WithError(err).Error("connection ended with error")
		return err
	}

	return nil
}

// deviceStreamImpl wraps connect.BidiStream to implement business.DeviceStream.
type deviceStreamImpl struct {
	stream *connect.BidiStream[chatv1.StreamRequest, chatv1.StreamResponse]
}

func (w *deviceStreamImpl) Receive() (*chatv1.StreamRequest, error) {
	return w.stream.Receive()
}

func (w *deviceStreamImpl) Send(event *chatv1.StreamResponse) error {
	return w.stream.Send(event)
}
