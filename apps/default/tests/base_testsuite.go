package tests

import (
	"context"
	"errors"
	"testing"

	"buf.build/gen/go/antinvestor/device/connectrpc/go/device/v1/devicev1connect"
	devicev1 "buf.build/gen/go/antinvestor/device/protocolbuffers/go/device/v1"
	"buf.build/gen/go/antinvestor/notification/connectrpc/go/notification/v1/notificationv1connect"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	profilev1 "buf.build/gen/go/antinvestor/profile/protocolbuffers/go/profile/v1"
	"connectrpc.com/connect"
	devicemocks "github.com/antinvestor/apis/go/device/mocks"
	notificationmocks "github.com/antinvestor/apis/go/notification/mocks"
	profilemocks "github.com/antinvestor/apis/go/profile/mocks"
	iconfig "github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/gojuno/minimock/v3"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/config"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/workerpool"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
)

const PostgresqlDBImage = "postgres:latest"

const (
	DefaultRandomStringLength = 8
)

type BaseTestSuite struct {
	frametests.FrameBaseTestSuite
}

func initResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("service_chat",
		definition.WithUserName("ant"),
		definition.WithImageName(PostgresqlDBImage),
		definition.WithEnableLogging(true))
	resources := []definition.TestResource{pg}
	return resources
}

func (bs *BaseTestSuite) SetupSuite() {
	bs.InitResourceFunc = initResources
	bs.FrameBaseTestSuite.SetupSuite()
}

func (bs *BaseTestSuite) CreateService(
	t *testing.T,
	depOpts *definition.DependencyOption,
) (context.Context, *frame.Service) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	ctx := t.Context()
	cfg, err := config.FromEnv[iconfig.ChatConfig]()
	require.NoError(t, err)

	cfg.LogLevel = "debug"
	cfg.DatabaseMigrate = true
	cfg.RunServiceSecurely = false
	cfg.ServerPort = ""

	res := depOpts.ByIsDatabase(ctx)
	testDS, cleanup, err0 := res.GetRandomisedDS(t.Context(), depOpts.Prefix())
	require.NoError(t, err0)

	t.Cleanup(func() {
		cleanup(t.Context())
	})

	cfg.DatabaseTraceQueries = true
	cfg.DatabasePrimaryURL = []string{testDS.String()}
	cfg.DatabaseReplicaURL = []string{testDS.String()}

	ctx, svc := frame.NewServiceWithContext(t.Context(),
		frame.WithName("chat tests"),
		frame.WithConfig(&cfg),
		frametests.WithNoopDriver(),
		frame.WithDatastore())

	dbManager := svc.DatastoreManager()
	workMan := svc.WorkManager()
	eventsMan := svc.EventsManager()
	queueMan := svc.QueueManager()

	dbPool := dbManager.GetPool(ctx, datastore.DefaultPoolName)

	err = repository.Migrate(ctx, dbManager, "../../migrations/0001")
	require.NoError(t, err)

	eventDeliveryQueuePublisher := frame.WithRegisterPublisher(
		cfg.QueueDeviceEventDeliveryName,
		cfg.QueueDeviceEventDeliveryURI,
	)

	eventDeliveryQueueSubscriber := frame.WithRegisterSubscriber(
		cfg.QueueDeviceEventDeliveryName,
		cfg.QueueDeviceEventDeliveryURI,
		queues.NewHotPathDeliveryQueueHandler(&cfg, queueMan, workMan, bs.GetDevice(t)),
	)

	// Register queue handlers and event handlers
	serviceOptions := []frame.Option{
		eventDeliveryQueuePublisher,
		eventDeliveryQueueSubscriber,
		frame.WithRegisterEvents(
			events.NewRoomOutboxLoggingQueue(ctx, dbPool, workMan, eventsMan),
			events.NewFanoutEventHandler(ctx, &cfg, dbPool, workMan, queueMan),
		)}

	// Initialize the service with all options
	svc.Init(ctx, serviceOptions...)

	// Run the service
	err = svc.Run(ctx, "")
	require.NoError(t, err)

	return ctx, svc
}

// GetRepoDeps is a helper to create repository dependencies.
func (bs *BaseTestSuite) GetRepoDeps(ctx context.Context, svc *frame.Service) (workerpool.Manager, pool.Pool) {
	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	return workMan, dbPool
}

func (bs *BaseTestSuite) GetNotificationCli(t *testing.T) notificationv1connect.NotificationServiceClient {
	ctrl := minimock.NewController(t)

	mocksvc := notificationmocks.NewNotificationServiceClientMock(ctrl)
	return mocksvc
}

func (bs *BaseTestSuite) GetProfileCli(t *testing.T) profilev1connect.ProfileServiceClient {
	ctrl := minimock.NewController(t)
	mocksvc := profilemocks.NewProfileServiceClientMock(ctrl)

	return mocksvc
}

func (bs *BaseTestSuite) GetDevice(t *testing.T) devicev1connect.DeviceServiceClient {
	ctrl := minimock.NewController(t)
	mockSvc := devicemocks.NewDeviceServiceClientMock(ctrl)

	// Configure the mock to expect Search calls and return no devices found error
	mockSvc.SearchMock.Optional().
		Set(func(_ context.Context, _ *connect.Request[devicev1.SearchRequest]) (*connect.ServerStreamForClient[devicev1.SearchResponse], error) {
			return nil, errors.New("no devices found")
		})

	return mockSvc
}

func (bs *BaseTestSuite) CreateTestProfiles(
	_ context.Context,
	_ *frame.Service,
	contacts []string,
) ([]*profilev1.ProfileObject, error) {
	// Mock profile creation for testing
	var profileSlice []*profilev1.ProfileObject

	for range contacts {
		profile := &profilev1.ProfileObject{
			Id: util.IDString(),
		}
		profileSlice = append(profileSlice, profile)
	}

	return profileSlice, nil
}

func (bs *BaseTestSuite) TearDownSuite() {
	bs.FrameBaseTestSuite.TearDownSuite()
}

// WithTestDependencies Creates subtests with each known DependencyOption.
func (bs *BaseTestSuite) WithTestDependencies(
	t *testing.T,
	testFn func(t *testing.T, dep *definition.DependencyOption),
) {
	options := []*definition.DependencyOption{
		definition.NewDependancyOption(
			"default",
			util.RandomAlphaNumericString(DefaultRandomStringLength),
			bs.Resources(),
		),
	}

	frametests.WithTestDependencies(t, options, testFn)
}

// WithAuthClaims adds authentication claims to a context for testing.
func (bs *BaseTestSuite) WithAuthClaims(ctx context.Context, profileID string) context.Context {
	claims := &security.AuthenticationClaims{
		TenantID:  util.IDString(),
		AccessID:  util.IDString(),
		ContactID: profileID,
		SessionID: util.IDString(),
		DeviceID:  "test-device",
	}
	// Set the Subject field from jwt.RegisteredClaims
	claims.Subject = profileID
	return claims.ClaimsToContext(ctx)
}
