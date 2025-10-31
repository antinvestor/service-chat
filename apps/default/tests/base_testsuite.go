package tests

import (
	"context"
	"testing"

	"github.com/antinvestor/apis/go/common"
	"github.com/antinvestor/apis/go/common/mocks"
	commonv1 "github.com/antinvestor/apis/go/common/v1"
	devicev1 "github.com/antinvestor/apis/go/device/v1"
	devicev1_mocks "github.com/antinvestor/apis/go/device/v1_mocks"
	notificationv1 "github.com/antinvestor/apis/go/notification/v1"
	notificationv1_mocks "github.com/antinvestor/apis/go/notification/v1_mocks"
	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	profilev1_mocks "github.com/antinvestor/apis/go/profile/v1_mocks"
	iconfig "github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/queues"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
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
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
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

	ctx, svc := frame.NewServiceWithContext(t.Context(), "chat tests",
		frame.WithConfig(&cfg),
		frametests.WithNoopDriver())

	// Initialize service with datastore : pool.WithTraceConfig(&cfg)
	svc.Init(ctx, frame.WithDatastore())

	var serviceOptions []frame.Option

	// Get pool and migrate
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	migrationPool := pool.NewPool(ctx)
	err = migrationPool.AddConnection(ctx, pool.WithConnection(testDS.String(), false), pool.WithPreparedStatements(false))
	require.NoError(t, err)

	err = repository.Migrate(ctx, migrationPool, "../../migrations/0001")
	require.NoError(t, err)

	EventDeliveryQueuePublisher := frame.WithRegisterPublisher(
		cfg.QueueUserEventDeliveryName,
		cfg.QueueUserEventDeliveryURI,
	)

	EventDeliveryQueueSubscriber := frame.WithRegisterSubscriber(
		cfg.QueueUserEventDeliveryName,
		cfg.QueueUserEventDeliveryURI,
		queues.NewEventDeliveryQueueHandler(svc, bs.GetDevice(ctx)),
	)

	// Get publisher for event handlers
	deliveryPublisher, _ := svc.QueueManager(ctx).GetPublisher(cfg.QueueUserEventDeliveryName)
	workMan := svc.WorkManager()

	// Register queue handlers and event handlers
	serviceOptions = append(serviceOptions,
		EventDeliveryQueuePublisher, EventDeliveryQueueSubscriber,
		frame.WithRegisterEvents(
			events.NewRoomOutboxLoggingQueue(ctx, svc, dbPool, workMan),
			events.NewOutboxDeliveryEventHandler(ctx, dbPool, workMan, deliveryPublisher),
		))

	// Initialize the service with all options
	svc.Init(ctx, serviceOptions...)

	// Run the service
	err = svc.Run(ctx, "")
	require.NoError(t, err)

	return ctx, svc
}

// GetRepoDeps is a helper to create repository dependencies
func (bs *BaseTestSuite) GetRepoDeps(ctx context.Context, svc *frame.Service) (workerpool.Manager, pool.Pool) {
	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	return workMan, dbPool
}

func (bs *BaseTestSuite) GetNotificationCli(_ context.Context) *notificationv1.NotificationClient {
	mockSvc := notificationv1_mocks.NewMockNotificationServiceClient(bs.Ctrl)
	mockSvc.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *notificationv1.SendRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[notificationv1.SendResponse], error) {
			// Return a successful response with a generated message ID
			const randomIDLength = 6
			resp := &notificationv1.SendResponse{
				Data: []*commonv1.StatusResponse{
					{
						Id:         util.IDString(),
						State:      commonv1.STATE_ACTIVE,
						Status:     commonv1.STATUS_SUCCESSFUL,
						ExternalId: util.RandomString(randomIDLength),
					},
				},
			}

			// Create a custom mock implementation
			mockStream := mocks.NewMockServerStreamingClient[notificationv1.SendResponse](ctx)
			err := mockStream.SendMsg(resp)
			if err != nil {
				return nil, err
			}

			return mockStream, nil
		}).
		AnyTimes()
	cli := notificationv1.Init(&common.GrpcClientBase{}, mockSvc)

	return cli
}

func (bs *BaseTestSuite) GetProfileCli(_ context.Context) *profilev1.ProfileClient {
	mockProfileService := profilev1_mocks.NewMockProfileServiceClient(bs.Ctrl)

	cli := profilev1.Init(&common.GrpcClientBase{}, mockProfileService)

	return cli
}

func (bs *BaseTestSuite) GetDevice(_ context.Context) *devicev1.DeviceClient {
	mockSvc := devicev1_mocks.NewMockDeviceServiceClient(bs.Ctrl)

	cli := devicev1.Init(&common.GrpcClientBase{}, mockSvc)

	return cli
}

func (bs *BaseTestSuite) CreateTestProfiles(
	ctx context.Context,
	svc *frame.Service,
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
		definition.NewDependancyOption("default", util.RandomString(DefaultRandomStringLength), bs.Resources()),
	}

	frametests.WithTestDependencies(t, options, testFn)
}

// WithAuthClaims adds authentication claims to a context for testing
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
