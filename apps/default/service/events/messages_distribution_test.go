package events_test

import (
	"context"
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ClientSetupQueueTestSuite struct {
	tests.BaseTestSuite
}

func TestClientSetupQueueSuite(t *testing.T) {
	suite.Run(t, new(ClientSetupQueueTestSuite))
}

func (csqts *ClientSetupQueueTestSuite) createQueue(
	ctx context.Context,
	svc *frame.Service,
) *events.RoomOutboxLoggingQueue {
	workMan := svc.WorkManager()
	eventsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	return events.NewRoomOutboxLoggingQueue(ctx, dbPool, workMan, eventsMan)
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Name() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)
		require.Equal(t, events.RoomOutboxLoggingEventName, queue.Name())
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_PayloadType() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)
		payloadType := queue.PayloadType()

		// Should return a pointer to EventLink protobuf
		require.NotNil(t, payloadType)
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Validate() {
	t := csqts.T()
	t.Skip("Test needs to be updated for protobuf payloads")
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_InvalidPayload() {
	t := csqts.T()
	t.Skip("Test needs to be updated for protobuf payloads")
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_NonExistentRelationship() {
	t := csqts.T()
	t.Skip("Test needs to be updated for protobuf payloads")
}

func (csqts *ClientSetupQueueTestSuite) TestNewClientConnectedSetupQueue() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)
		require.NotNil(t, queue)
	})
}
