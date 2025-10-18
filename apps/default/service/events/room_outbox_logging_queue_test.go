package events_test

import (
	"testing"

	profilev1 "github.com/antinvestor/apis/go/profile/v1"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
)

type ClientSetupQueueTestSuite struct {
	tests.BaseTestSuite
}

func TestClientSetupQueueSuite(t *testing.T) {
	suite.Run(t, new(ClientSetupQueueTestSuite))
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Name() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, _ := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)
		require.Equal(t, events.RoomOutboxLoggingQueueName, queue.Name())
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_PayloadType() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, _ := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)
		payloadType := queue.PayloadType()

		// Should return a pointer to string
		_, ok := payloadType.(*string)
		require.True(t, ok, "PayloadType should return *string")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Validate() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)

		// Test valid payload
		validPayload := "test-relationship-id"
		err := queue.Validate(ctx, &validPayload)
		require.NoError(t, err)

		// Test invalid payload type
		invalidPayload := 123
		err = queue.Validate(ctx, invalidPayload)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid payload type, expected *string")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_Success() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := csqts.CreateService(t, dep)

		// Create test data
		profileBusiness := business.NewProfileBusiness(ctx, svc)

		// Create a test relationship
		relationship := &models.Relationship{
			ParentObject:   "profile",
			ParentObjectID: util.IDString(),
			ChildObject:    "profile",
			ChildObjectID:  util.IDString(),
		}
		relationship.GenID(ctx)

		// Save relationship to database
		relationshipRepo := repository.NewRelationshipRepository(svc)

		relationshipType, err := relationshipRepo.RelationshipType(ctx, profilev1.RelationshipType_MEMBER)
		require.NoError(t, err)

		relationship.RelationshipTypeID = relationshipType.GetID()

		err = relationshipRepo.Save(ctx, relationship)
		require.NoError(t, err)

		queue := events.NewRoomOutboxLoggingQueue(svc)
		relationshipID := relationship.GetID()

		// Execute the queue handler
		err = queue.Execute(ctx, &relationshipID)
		require.NoError(t, err)

		_ = profileBusiness
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_InvalidPayload() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)

		// Test with invalid payload type
		invalidPayload := 123
		err := queue.Execute(ctx, invalidPayload)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid payload type, expected *string")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_NonExistentRelationship() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)
		nonExistentID := util.IDString()

		// Execute with non-existent relationship ID - should not return error (logs and continues)
		err := queue.Execute(ctx, &nonExistentID)
		require.NoError(t, err, "Should handle non-existent relationship gracefully")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestNewClientConnectedSetupQueue() {
	t := csqts.T()

	csqts.WithTestDependancies(t, func(t *testing.T, dep *definition.DependancyOption) {
		svc, _ := csqts.CreateService(t, dep)

		queue := events.NewRoomOutboxLoggingQueue(svc)
		require.NotNil(t, queue)
		require.Equal(t, svc, queue.Service)
	})
}
