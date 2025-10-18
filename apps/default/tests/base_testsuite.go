package tests

import (
	"context"
	"testing"

	"github.com/antinvestor/apis/go/common"
	"github.com/antinvestor/apis/go/common/mocks"
	commonv1 "github.com/antinvestor/apis/go/common/v1"
	notificationv1 "github.com/antinvestor/apis/go/notification/v1"
	notificationv1_mocks "github.com/antinvestor/apis/go/notification/v1_mocks"
	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/frametests/deps/testpostgres"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"

	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
)

const PostgresqlDBImage = "postgres:latest"

const (
	DefaultRandomStringLength = 8
)

type BaseTestSuite struct {
	frametests.FrameBaseTestSuite
}

func initResources(_ context.Context) []definition.TestResource {
	pg := testpostgres.NewWithOpts("service_profile", definition.WithUserName("ant"))
	resources := []definition.TestResource{pg}
	return resources
}

func (bs *BaseTestSuite) SetupSuite() {
	bs.InitResourceFunc = initResources
	bs.FrameBaseTestSuite.SetupSuite()
}

func (bs *BaseTestSuite) CreateService(
	t *testing.T,
	depOpts *definition.DependancyOption,
) (*frame.Service, context.Context) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	ctx := t.Context()
	profileConfig, err := frame.ConfigFromEnv[config.ProfileConfig]()
	require.NoError(t, err)

	profileConfig.LogLevel = "debug"
	profileConfig.RunServiceSecurely = false
	profileConfig.ServerPort = ""

	res := depOpts.ByIsDatabase(ctx)
	testDS, cleanup, err0 := res.GetRandomisedDS(t.Context(), depOpts.Prefix())
	require.NoError(t, err0)

	t.Cleanup(func() {
		cleanup(t.Context())
	})

	profileConfig.DatabasePrimaryURL = []string{testDS.String()}
	profileConfig.DatabaseReplicaURL = []string{testDS.String()}

	ctx, svc := frame.NewServiceWithContext(t.Context(), "profile tests",
		frame.WithConfig(&profileConfig),
		frame.WithDatastore(),
		frametests.WithNoopDriver())

	relationshipConnectQueuePublisher := frame.WithRegisterPublisher(
		profileConfig.QueueRelationshipConnectName,
		profileConfig.QueueRelationshipConnectURI,
	)
	relationshipDisConnectQueuePublisher := frame.WithRegisterPublisher(
		profileConfig.QueueRelationshipDisConnectName,
		profileConfig.QueueRelationshipDisConnectURI,
	)

	svc.Init(ctx,
		relationshipConnectQueuePublisher, relationshipDisConnectQueuePublisher,
		frame.WithRegisterEvents(
			events.NewRoomOutboxLoggingQueue(svc),
		),
	)

	err = repository.Migrate(ctx, svc, "../../migrations/0001")
	require.NoError(t, err)

	err = svc.Run(ctx, "")
	require.NoError(t, err)

	return svc, ctx
}

func (bs *BaseTestSuite) GetNotificationCli(_ context.Context) *notificationv1.NotificationClient {
	mockNotificationService := notificationv1_mocks.NewMockNotificationServiceClient(bs.Ctrl)
	mockNotificationService.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(
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
	notificationCli := notificationv1.Init(&common.GrpcClientBase{}, mockNotificationService)

	return notificationCli
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

// WithTestDependancies Creates subtests with each known DependancyOption.
func (bs *BaseTestSuite) WithTestDependancies(
	t *testing.T,
	testFn func(t *testing.T, dep *definition.DependancyOption),
) {
	options := []*definition.DependancyOption{
		definition.NewDependancyOption("default", util.RandomString(DefaultRandomStringLength), bs.Resources()),
	}

	frametests.WithTestDependancies(t, options, testFn)
}
