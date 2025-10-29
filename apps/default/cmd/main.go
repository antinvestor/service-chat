package main

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"github.com/antinvestor/apis/go/chat/v1/chatv1connect"
	apis "github.com/antinvestor/apis/go/common"
	devicev1 "github.com/antinvestor/apis/go/device/v1"
	notificationv1 "github.com/antinvestor/apis/go/notification/v1"
	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	aconfig "github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/handlers"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/security"
	securityconnect "github.com/pitabwire/frame/security/interceptors/connect"
	"github.com/pitabwire/frame/security/openid"
	"github.com/pitabwire/util"
)

func main() {
	serviceName := "service_chat"
	ctx := context.Background()

	// Initialize configuration
	cfg, err := config.LoadWithOIDC[aconfig.ChatConfig](ctx)
	if err != nil {
		util.Log(ctx).With("err", err).Error("could not process configs")
		return
	}

	// Create service
	ctx, svc := frame.NewServiceWithContext(ctx, serviceName, frame.WithConfig(&cfg), frame.WithRegisterServerOauth2Client())
	defer svc.Stop(ctx)
	log := svc.Log(ctx)

	// Handle database migration if requested
	if handleDatabaseMigration(ctx, svc, cfg, log) {
		return
	}

	sm := svc.SecurityManager()

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

	// Setup Connect server
	connectHandler := setupConnectServer(ctx, svc, notificationCli, profileCli, cfg, serviceName, log)

	// Setup HTTP handlers and proxy
	serviceOptions, err := setupServiceOptions(ctx, connectHandler)
	if err != nil {
		log.WithError(err).Fatal("could not setup HTTP handlers")
	}

	userDeliveryQueuePublisher := frame.WithRegisterPublisher(
		cfg.QueueUserEventDeliveryName,
		cfg.QueueUserEventDeliveryURI,
	)
	userDeliveryQueueSubscriber := frame.WithRegisterSubscriber(
		cfg.QueueUserEventDeliveryName,
		cfg.QueueUserEventDeliveryURI,
		queues.NewUserDeliveryQueueHandler(svc, deviceCli),
	)
	
	// Get publisher for event handlers
	deliveryPublisher, _ := svc.QueueManager(ctx).GetPublisher(cfg.QueueUserEventDeliveryName)
	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	
	// Register queue handlers and event handlers
	serviceOptions = append(serviceOptions,
		userDeliveryQueuePublisher, userDeliveryQueueSubscriber,
		frame.WithRegisterEvents(
			events.NewRoomOutboxLoggingQueue(svc),
			events.NewOutboxDeliveryEventHandler(dbPool, workMan, deliveryPublisher),
		))

	// Initialize the service with all options
	svc.Init(ctx, serviceOptions...)

	// Start the service
	log.WithField("server http port", cfg.HTTPPort()).
		WithField("server grpc port", cfg.GrpcPort()).
		Info(" Initiating server operations")

	err = svc.Run(ctx, "")
	if err != nil {
		log.WithError(err).Fatal("could not run Server")
	}
}

// handleDatabaseMigration performs database migration if configured to do so.
func handleDatabaseMigration(
	ctx context.Context,
	svc *frame.Service,
	cfg aconfig.ChatConfig,
	log *util.LogEntry,
) bool {
	serviceOptions := []frame.Option{frame.WithDatastore()}

	if !cfg.DoDatabaseMigrate() {
		return false
	}
	svc.Init(ctx, serviceOptions...)

	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	err := repository.Migrate(ctx, dbPool, cfg.GetDatabaseMigrationPath())
	if err != nil {
		log.WithError(err).Fatal("main -- Could not migrate successfully")
	}
	return true

}

// setupNotificationClient creates and configures the notification client.
func setupNotificationClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (*notificationv1.NotificationClient, error) {
	return notificationv1.NewNotificationClient(ctx,
		apis.WithEndpoint(cfg.NotificationServiceURI),
		apis.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		apis.WithTokenUsername(clHolder.JwtClientID()),
		apis.WithTokenPassword(clHolder.JwtClientSecret()),
		apis.WithScopes(openid.ConstSystemScopeInternal),
		apis.WithAudiences("service_notifications"))
}

// setupProfileClient creates and configures the profile client.
func setupProfileClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (*profilev1.ProfileClient, error) {
	return profilev1.NewProfileClient(ctx,
		apis.WithEndpoint(cfg.ProfileServiceURI),
		apis.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		apis.WithTokenUsername(clHolder.JwtClientID()),
		apis.WithTokenPassword(clHolder.JwtClientSecret()),
		apis.WithScopes(openid.ConstSystemScopeInternal),
		apis.WithAudiences("service_profile"))
}

// setupDeviceClient creates and configures the device client.
func setupDeviceClient(
	ctx context.Context,
	clHolder security.InternalOauth2ClientHolder,
	cfg aconfig.ChatConfig) (*devicev1.DeviceClient, error) {
	return devicev1.NewDeviceClient(ctx,
		apis.WithEndpoint(cfg.DeviceServiceURI),
		apis.WithTokenEndpoint(cfg.GetOauth2TokenEndpoint()),
		apis.WithTokenUsername(clHolder.JwtClientID()),
		apis.WithTokenPassword(clHolder.JwtClientSecret()),
		apis.WithScopes(openid.ConstSystemScopeInternal),
		apis.WithAudiences("service_device"))
}

// setupConnectServer initializes and configures the gRPC server.
func setupConnectServer(ctx context.Context, svc *frame.Service,
	notificationCli *notificationv1.NotificationClient,
	profileCli *profilev1.ProfileClient,
	cfg aconfig.ChatConfig,
	serviceName string,
	log *util.LogEntry) http.Handler {

	securityMan := svc.SecurityManager()

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		log.WithError(err).Fatal("could not configure open telemetry")
	}

	validateInterceptor, err := securityconnect.NewValidationInterceptor()
	if err != nil {
		log.WithError(err).Fatal("could not configure validation interceptor")
	}

	authInterceptor := securityconnect.NewAuthInterceptor(securityMan.GetAuthenticator(ctx))

	implementation := handlers.NewChatServer(ctx, svc, notificationCli, profileCli)

	_, serverHandler := chatv1connect.NewChatServiceHandler(
		implementation, connect.WithInterceptors(authInterceptor, otelInterceptor, validateInterceptor))

	return serverHandler
}

// setupServiceOptions configures HTTP handlers and proxy.
func setupServiceOptions(
	_ context.Context,
	serverHandler http.Handler,
) ([]frame.Option, error) {
	// Start with datastore option
	serviceOptions := []frame.Option{frame.WithDatastore()}

	serviceOptions = append(serviceOptions, frame.WithHTTPHandler(serverHandler))

	return serviceOptions, nil
}
