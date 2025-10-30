package main

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/antinvestor/apis/go/chat/v1/chatv1connect"
	apis "github.com/antinvestor/apis/go/common"
	devicev1 "github.com/antinvestor/apis/go/device/v1"
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
	serviceName := "service_chat_gateway"
	ctx := context.Background()

	// Initialize configuration
	cfg, err := config.LoadWithOIDC[gtwconfig.GatewayConfig](ctx)
	if err != nil {
		util.Log(ctx).With("err", err).Error("could not process configs")
		return
	}

	// Create service
	ctx, svc := frame.NewServiceWithContext(ctx, serviceName, frame.WithConfig(&cfg), frame.WithRegisterServerOauth2Client())
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
	httpClient, err := apis.HTTPClient(ctx,
		apis.WithEndpoint(cfg.ChatServiceURI),
		apis.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		apis.WithTokenUsername(sm.JwtClientID()),
		apis.WithTokenPassword(sm.JwtClientSecret()),
		apis.WithScopes(openid.ConstSystemScopeInternal),
		apis.WithAudiences("service_chat"))
	if err != nil {
		return nil, err
	}

	// Create chat service client
	client := chatv1connect.NewChatServiceClient(
		httpClient,
		cfg.ChatServiceURI,
	)

	return client, nil
}

// setupDeviceClient creates and configures the device service client.
func setupDeviceClient(
	ctx context.Context,
	sm security.Manager,
	cfg gtwconfig.GatewayConfig,
) (*devicev1.DeviceClient, error) {
	return devicev1.NewDeviceClient(ctx,
		apis.WithEndpoint(cfg.DeviceServiceURI),
		apis.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		apis.WithTokenUsername(sm.JwtClientID()),
		apis.WithTokenPassword(sm.JwtClientSecret()),
		apis.WithScopes(openid.ConstSystemScopeInternal),
		apis.WithAudiences("service_device"))
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
