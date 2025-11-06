package main

import (
	"context"
	"net/http"

	"buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/antinvestor/apis/go/chat"
	"github.com/antinvestor/apis/go/common"
	"github.com/antinvestor/apis/go/device"
	gtwconfig "github.com/antinvestor/service-chat/apps/gateway/config"
	"github.com/antinvestor/service-chat/apps/gateway/service/business"
	"github.com/antinvestor/service-chat/apps/gateway/service/handlers"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/cache/valkey"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/security"
	securityconnect "github.com/pitabwire/frame/security/interceptors/connect"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/util"
)

func main() {
	ctx := context.Background()

	// Initialize configuration
	cfg, err := config.LoadWithOIDC[gtwconfig.GatewayConfig](ctx)
	if err != nil {
		util.Log(ctx).With("err", err).Error("could not process configs")
		return
	}

	if cfg.Name() == "" {
		cfg.ServiceName = "service_chat_gateway"
	}

	// Create service
	ctx, svc := frame.NewServiceWithContext(ctx, frame.WithConfig(&cfg), frame.WithRegisterServerOauth2Client())
	defer svc.Stop(ctx)
	log := svc.Log(ctx)

	// Setup chat service client
	chatServiceClient, err := setupChatServiceClient(ctx, svc.SecurityManager(), cfg)
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup chat service client")
	}

	// Setup device service client for delivery tracking
	deviceClient, err := setupDeviceClient(ctx, svc.SecurityManager(), cfg)
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup device service client")
	}

	// Setup cache for connection metadata
	defaultCache, err := valkey.New(cache.WithDSN(data.DSN(cfg.CacheURI)))
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup default cache")
	}

	// Setup connection manager with clients
	// Note: Outbound message delivery is handled by the default service via direct publish
	// The gateway focuses on inbound request processing and real-time features
	connectionManager := business.NewConnectionManager(
		ctx,
		chatServiceClient,
		deviceClient,
		cfg.MaxConnectionsPerDevice,
		cfg.ConnectionTimeoutSec,
		cfg.HeartbeatIntervalSec,
	)

	serviceOptions := []frame.Option{
		frame.WithCache(cfg.CacheName, defaultCache),
	}

	// Setup gateway server
	gatewayHandler := setupGatewayServer(ctx, svc, chatServiceClient, connectionManager)

	serviceOptions = append(serviceOptions, frame.WithHTTPHandler(gatewayHandler))

	// Initialize the service with all options
	svc.Init(ctx, serviceOptions...)

	// Start the service
	log.WithField("server http port", cfg.HTTPPort()).
		WithField("server grpc port", cfg.GrpcPort()).
		Info(" Initiating gateway server operations")

	err = svc.Run(ctx, "")
	if err != nil {
		log.WithError(err).Fatal("could not run Server")
	}
}

// setupChatServiceClient creates and configures the chat service client.
func setupChatServiceClient(
	ctx context.Context,
	sm security.Manager,
	cfg gtwconfig.GatewayConfig,
) (chatv1connect.ChatServiceClient, error) {
	// Create HTTP client for the chat service
	return chat.NewClient(ctx,
		common.WithEndpoint(cfg.ChatServiceURI),
		common.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		common.WithTokenUsername(sm.JwtClientID()),
		common.WithTokenPassword(sm.JwtClientSecret()),
		common.WithScopes(openid.ConstSystemScopeInternal),
		common.WithAudiences("service_chat"))
}

// setupDeviceClient creates and configures the device service client.
func setupDeviceClient(
	ctx context.Context,
	sm security.Manager,
	cfg gtwconfig.GatewayConfig,
) (devicev1connect.DeviceServiceClient, error) {
	return device.NewClient(ctx,
		common.WithEndpoint(cfg.DeviceServiceURI),
		common.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		common.WithTokenUsername(sm.JwtClientID()),
		common.WithTokenPassword(sm.JwtClientSecret()),
		common.WithScopes(openid.ConstSystemScopeInternal),
		common.WithAudiences("service_device"))
}

// setupGatewayServer initializes and configures the gateway server.
func setupGatewayServer(
	ctx context.Context,
	svc *frame.Service,
	chatServiceClient chatv1connect.ChatServiceClient,
	connectionManager business.ConnectionManager,
) http.Handler {
	securityMan := svc.SecurityManager()

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		util.Log(ctx).WithError(err).Fatal("could not configure open telemetry")
	}

	validateInterceptor, err := securityconnect.NewValidationInterceptor()
	if err != nil {
		util.Log(ctx).WithError(err).Fatal("could not configure validation interceptor")
	}

	authInterceptor := securityconnect.NewAuthInterceptor(securityMan.GetAuthenticator(ctx))

	gatewayServer := handlers.NewGatewayServer(svc, chatServiceClient, connectionManager)

	// Register as GatewayServiceHandler - handles the Connect stream for real-time communication
	_, serverHandler := chatv1connect.NewGatewayServiceHandler(
		gatewayServer, connect.WithInterceptors(authInterceptor, otelInterceptor, validateInterceptor),
	)

	return serverHandler
}
