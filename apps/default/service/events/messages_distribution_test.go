package events_test

import (
	"context"
	"testing"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/assert"
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
		_, ok := payloadType.(*eventsv1.Link)
		assert.True(t, ok, "PayloadType should return *eventsv1.Link")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Validate_ValidPayload() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)

		// Valid payload
		payload := &eventsv1.Link{
			EventId: "event123",
			RoomId:  "room456",
		}

		err := queue.Validate(ctx, payload)
		assert.NoError(t, err)
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Validate_InvalidPayload() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)

		// Invalid payload type
		err := queue.Validate(ctx, "invalid string payload")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid payload type")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_InvalidPayload() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)

		// Invalid payload type
		err := queue.Execute(ctx, "invalid string payload")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid payload type")
	})
}

func (csqts *ClientSetupQueueTestSuite) TestClientConnectedSetupQueue_Execute_NonExistentRoom() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)

		// Valid payload with non-existent room
		payload := &eventsv1.Link{
			EventId: "event123",
			RoomId:  "nonexistent-room",
		}

		// Should return nil when no subscribers found (not an error)
		err := queue.Execute(ctx, payload)
		assert.NoError(t, err)
	})
}

func (csqts *ClientSetupQueueTestSuite) TestNewClientConnectedSetupQueue() {
	t := csqts.T()

	csqts.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := csqts.CreateService(t, dep)

		queue := csqts.createQueue(ctx, svc)
		require.NotNil(t, queue)
	})
}
