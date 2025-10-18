package events_test

import (
	"testing"

	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/antinvestor/service-chat/apps/default/service/events"
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
