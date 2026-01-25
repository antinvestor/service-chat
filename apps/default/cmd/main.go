package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"buf.build/gen/go/antinvestor/chat/connectrpc/go/chat/v1/chatv1connect"
	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	"buf.build/gen/go/antinvestor/notification/connectrpc/go/notification/v1/notificationv1connect"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/antinvestor/apis/go/common"
	"github.com/antinvestor/apis/go/device"
	"github.com/antinvestor/apis/go/notification"
	"github.com/antinvestor/apis/go/profile"
	aconfig "github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/antinvestor/service-chat/apps/default/service/authz/keto"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/handlers"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/internal/health"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/security"
	securityconnect "github.com/pitabwire/frame/security/interceptors/connect"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/util"
)

// runService initializes and starts the chat service with all dependencies.
func runService(ctx context.Context) error {
	// Initialize configuration
	cfg, err := config.LoadWithOIDC[aconfig.ChatConfig](ctx)
	if err != nil {
		util.Log(ctx).With("err", err).Error("could not process configs")
		return err
	}

	if cfg.Name() == "" {
		cfg.ServiceName = "service_chat"
	}

	// Create service
	ctx, svc := frame.NewServiceWithContext(
		ctx,
		frame.WithConfig(&cfg),
		frame.WithRegisterServerOauth2Client(),
		frame.WithDatastore(),
	)
	defer svc.Stop(ctx)
	log := svc.Log(ctx)

	sm := svc.SecurityManager()

	// Get publisher for event handlers
	workMan := svc.WorkManager()
	eventsMan := svc.EventsManager()
	queueMan := svc.QueueManager()

	dbManager := svc.DatastoreManager()
	dbPool := dbManager.GetPool(ctx, datastore.DefaultPoolName)

	// Setup clients and services
	deviceCli, err := setupDeviceClient(ctx, sm, cfg)
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup device client")
	}

	notificationCli, err := setupNotificationClient(ctx, sm, cfg)
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup notification client")
	}

	profileCli, err := setupProfileClient(ctx, sm, cfg)
	if err != nil {
		log.WithError(err).Fatal("main -- Could not setup profile client")
	}

	// Handle database migration if requested
	if handleDatabaseMigration(ctx, dbManager, cfg) {
		return nil
	}

	// Setup Keto authorization service
	authzMiddleware := setupAuthzMiddleware(ctx, cfg)

	// Setup Connect server
	connectHandler := setupConnectServer(ctx, svc, notificationCli, profileCli, authzMiddleware)

	// Setup health checks
	healthHandler := setupHealthChecks(ctx, dbPool)

	// Create multiplexer for HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler.LivenessHandler)
	mux.HandleFunc("/readyz", healthHandler.ReadinessHandler)
	mux.Handle("/", connectHandler)

	// Setup HTTP handlers
	// Start with datastore option
	serviceOptions := []frame.Option{frame.WithHTTPHandler(mux)}

	eventDeliveryQueuePublisher := frame.WithRegisterPublisher(
		cfg.QueueDeviceEventDeliveryName,
		cfg.QueueDeviceEventDeliveryURI,
	)
	serviceOptions = append(serviceOptions, eventDeliveryQueuePublisher)

	eventDeliveryQueueSubscriber := frame.WithRegisterSubscriber(
		cfg.QueueDeviceEventDeliveryName,
		cfg.QueueDeviceEventDeliveryURI,
		queues.NewHotPathDeliveryQueueHandler(&cfg, queueMan, workMan, deviceCli),
	)
	serviceOptions = append(serviceOptions, eventDeliveryQueueSubscriber)

	offlineDeliveryQueuePublisher := frame.WithRegisterPublisher(
		cfg.QueueOfflineEventDeliveryName,
		cfg.QueueOfflineEventDeliveryURI,
	)
	serviceOptions = append(serviceOptions, offlineDeliveryQueuePublisher)

	offlineDeliveryQueueSubscriber := frame.WithRegisterSubscriber(
		cfg.QueueOfflineEventDeliveryName,
		cfg.QueueOfflineEventDeliveryURI,
		queues.NewOfflineDeliveryQueueHandler(&cfg, deviceCli),
	)
	serviceOptions = append(serviceOptions, offlineDeliveryQueueSubscriber)

	for i := range cfg.ShardCount {
		gatewayQueueName := fmt.Sprintf(cfg.QueueGatewayEventDeliveryName, i)
		gatewayQueueURI := cfg.QueueGatewayEventDeliveryURI[i]

		gatewayQueuePublisher := frame.WithRegisterPublisher(
			gatewayQueueName,
			gatewayQueueURI,
		)
		serviceOptions = append(serviceOptions, gatewayQueuePublisher)
	}

	// Register queue handlers and event handlers
	serviceOptions = append(serviceOptions,
		frame.WithRegisterEvents(
			events.NewRoomOutboxLoggingQueue(ctx, dbPool, workMan, eventsMan),
			events.NewFanoutEventHandler(ctx, &cfg, dbPool, workMan, queueMan),
		))

	// Initialize the service with all options
	svc.Init(ctx, serviceOptions...)

	// Start the service
	return svc.Run(ctx, "")
}

func main() {
	ctx := context.Background()
	if err := runService(ctx); err != nil {
		util.Log(ctx).WithError(err).Fatal("could not run service")
	}
}

// setupHealthChecks creates the health check handler with database checker.
func setupHealthChecks(_ context.Context, dbPool pool.Pool) *health.Handler {
	handler := health.NewHandler()

	// Add database health checker
	dbChecker := health.NewDatabaseChecker(dbPool, 5*time.Second)
	handler.AddChecker(dbChecker)

	return handler
}

// handleDatabaseMigration performs database migration if configured to do so.
func handleDatabaseMigration(
	ctx context.Context,
	dbManager datastore.Manager,
	cfg aconfig.ChatConfig,
) bool {
	if !cfg.DoDatabaseMigrate() {
		return false
	}

	err := repository.Migrate(ctx, dbManager, cfg.GetDatabaseMigrationPath())
	if err != nil {
		util.Log(ctx).WithError(err).Fatal("main -- Could not migrate successfully")
	}
	return true
}

// setupNotificationClient creates and configures the notification client.
func setupNotificationClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (notificationv1connect.NotificationServiceClient, error) {
	return notification.NewClient(ctx,
		common.WithEndpoint(cfg.NotificationServiceURI),
		common.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		common.WithTokenUsername(clHolder.JwtClientID()),
		common.WithTokenPassword(clHolder.JwtClientSecret()),
		common.WithScopes(openid.ConstSystemScopeInternal),
		common.WithAudiences("service_notifications"))
}

// setupProfileClient creates and configures the profile client.
func setupProfileClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (profilev1connect.ProfileServiceClient, error) {
	return profile.NewClient(ctx,
		common.WithEndpoint(cfg.ProfileServiceURI),
		common.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		common.WithTokenUsername(clHolder.JwtClientID()),
		common.WithTokenPassword(clHolder.JwtClientSecret()),
		common.WithScopes(openid.ConstSystemScopeInternal),
		common.WithAudiences("service_profile"))
}

// setupDeviceClient creates and configures the device client.
func setupDeviceClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (devicev1connect.DeviceServiceClient, error) {
	return device.NewClient(ctx,
		common.WithEndpoint(cfg.DeviceServiceURI),
		common.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		common.WithTokenUsername(clHolder.JwtClientID()),
		common.WithTokenPassword(clHolder.JwtClientSecret()),
		common.WithScopes(openid.ConstSystemScopeInternal),
		common.WithAudiences("service_device"))
}

// setupAuthzMiddleware creates the authorization middleware with Keto adapter.
func setupAuthzMiddleware(ctx context.Context, cfg aconfig.ChatConfig) authz.AuthzMiddleware {
	ketoCfg := cfg.GetKetoConfig()

	// Create audit logger
	auditLogger := authz.NewAuditLogger(authz.AuditLoggerConfig{
		Enabled:    cfg.AuthzAuditEnabled,
		SampleRate: cfg.AuthzAuditSampleRate,
	})

	// Create Keto adapter
	ketoAdapter, err := keto.NewKetoAdapter(ketoCfg, auditLogger)
	if err != nil {
		util.Log(ctx).WithError(err).Warn("failed to initialize Keto adapter, using permissive mode")
		// Return nil middleware - the business layer will handle nil gracefully
		return nil
	}

	// Create and return middleware
	return authz.NewAuthzMiddleware(ketoAdapter, auditLogger)
}

// setupConnectServer initializes and configures the gRPC server.
func setupConnectServer(ctx context.Context, svc *frame.Service,
	notificationCli notificationv1connect.NotificationServiceClient,
	profileCli profilev1connect.ProfileServiceClient,
	authzMiddleware authz.AuthzMiddleware,
) http.Handler {
	securityMan := svc.SecurityManager()

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		util.Log(ctx).WithError(err).Fatal("could not configure open telemetry")
	}

	validateInterceptor := securityconnect.NewValidationInterceptor()

	authInterceptor := securityconnect.NewAuthInterceptor(securityMan.GetAuthenticator(ctx))

	implementation := handlers.NewChatServer(ctx, svc, notificationCli, profileCli, authzMiddleware)

	_, serverHandler := chatv1connect.NewChatServiceHandler(
		implementation, connect.WithInterceptors(authInterceptor, otelInterceptor, validateInterceptor))

	return serverHandler
}
