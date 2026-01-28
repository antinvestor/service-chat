package events_test

import (
	"context"
	"testing"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type FanoutEventHandlerTestSuite struct {
	tests.BaseTestSuite
}

func TestFanoutEventHandlerSuite(t *testing.T) {
	suite.Run(t, new(FanoutEventHandlerTestSuite))
}

func (s *FanoutEventHandlerTestSuite) createHandler(
	ctx context.Context,
	svc *frame.Service,
) *events.FanoutEventHandler {
	cfg := &config.ChatConfig{
		QueueDeviceEventDeliveryName: "test.delivery",
	}
	workMan := svc.WorkManager()
	queueMan := svc.QueueManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	return events.NewFanoutEventHandler(ctx, cfg, dbPool, workMan, queueMan)
}

func (s *FanoutEventHandlerTestSuite) TestName() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)
		assert.Equal(t, events.RoomFanoutEventName, handler.Name())
	})
}

func (s *FanoutEventHandlerTestSuite) TestPayloadType() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)
		payloadType := handler.PayloadType()

		require.NotNil(t, payloadType)
		_, ok := payloadType.(*eventsv1.Broadcast)
		assert.True(t, ok, "PayloadType should return *eventsv1.Broadcast")
	})
}

func (s *FanoutEventHandlerTestSuite) TestValidate_ValidPayload() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)

		broadcast := &eventsv1.Broadcast{
			Event: &eventsv1.Link{
				EventId: "event123",
				RoomId:  "room456",
			},
			Destinations: []*eventsv1.Subscription{
				{SubscriptionId: "sub1"},
			},
		}

		err := handler.Validate(ctx, broadcast)
		assert.NoError(t, err)
	})
}

func (s *FanoutEventHandlerTestSuite) TestValidate_InvalidPayload() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)

		// Invalid payload type
		err := handler.Validate(ctx, "invalid string payload")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid payload type")
	})
}

func (s *FanoutEventHandlerTestSuite) TestExecute_EmptyDestinations() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)

		broadcast := &eventsv1.Broadcast{
			Event: &eventsv1.Link{
				EventId: "event123",
				RoomId:  "room456",
			},
			Destinations: []*eventsv1.Subscription{}, // Empty destinations
		}

		// Should return nil (early exit) when no destinations
		err := handler.Execute(ctx, broadcast)
		assert.NoError(t, err)
	})
}

func (s *FanoutEventHandlerTestSuite) TestExecute_InvalidPayload() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)

		// Invalid payload type
		err := handler.Execute(ctx, "invalid string payload")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid payload type")
	})
}

func (s *FanoutEventHandlerTestSuite) TestExecute_EventNotFound() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)

		broadcast := &eventsv1.Broadcast{
			Event: &eventsv1.Link{
				EventId: "nonexistent-event",
				RoomId:  "room456",
			},
			Destinations: []*eventsv1.Subscription{
				{SubscriptionId: "sub1"},
			},
		}

		// Should return nil when event not found (logs error but doesn't fail)
		err := handler.Execute(ctx, broadcast)
		assert.NoError(t, err)
	})
}

func (s *FanoutEventHandlerTestSuite) TestNewFanoutEventHandler() {
	t := s.T()

	s.WithTestDependencies(t, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		handler := s.createHandler(ctx, svc)
		require.NotNil(t, handler)
	})
}
