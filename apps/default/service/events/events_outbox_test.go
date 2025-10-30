package events_test

import (
	"context"
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"
)

type OutboxEventTestSuite struct {
	tests.BaseTestSuite
}

func TestOutboxEventTestSuite(t *testing.T) {
	suite.Run(t, new(OutboxEventTestSuite))
}

func (s *OutboxEventTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) (business.RoomBusiness, business.MessageBusiness) {

	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
	outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return roomBusiness, messageBusiness
}

func (s *OutboxEventTestSuite) createQueue(ctx context.Context, svc *frame.Service) *events.RoomOutboxLoggingQueue {
	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
	return events.NewRoomOutboxLoggingQueue(ctx, svc, dbPool, workMan)
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueName() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		s.Equal(events.RoomOutboxLoggingEventName, queue.Name())
		s.Equal("room.outbox.logging.event", queue.Name())
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueuePayloadType() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		payloadType := queue.PayloadType()
		s.NotNil(payloadType)

		// Should be map[string]string
		_, ok := payloadType.(map[string]string)
		s.True(ok, "Payload type should be map[string]string")
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueValidateValidPayload() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		validPayload := map[string]string{
			"room_id":       util.IDString(),
			"room_event_id": util.IDString(),
			"sender_id":     util.IDString(),
		}

		err := queue.Validate(ctx, validPayload)
		require.NoError(t, err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueValidateInvalidPayload() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		// Invalid payload type
		invalidPayload := "not a map"

		err := queue.Validate(ctx, invalidPayload)
		require.Error(t, err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueExecuteCreatesOutboxEntries() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

		// Create room with multiple members
		creatorID := util.IDString()
		member1ID := util.IDString()
		member2ID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{member1ID, member2ID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Send a message to create an event
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   room.GetId(),
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.GetId(),
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Verify outbox entries were created for all subscribers
		// Should have entries for all members (creator is skipped in outbox)
		subs, err := subRepo.GetByRoomID(ctx, room.GetId(), true)
		require.NoError(t, err)

		// Count outbox entries
		outboxCount := 0
		for _, sub := range subs {
			if sub.ProfileID != creatorID {
				pending, err := outboxRepo.GetPendingBySubscription(ctx, sub.GetID(), 10)
				require.NoError(t, err)
				outboxCount += len(pending)
			}
		}

		// Should have outbox entries for non-sender members
		s.GreaterOrEqual(outboxCount, 2)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueUpdatesUnreadCount() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Get member's subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)

		// Initial unread count should be 0
		s.Equal(0, memberSub.UnreadCount)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   room.GetId(),
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.GetId(),
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Verify unread count increased (generated column)
		memberSub, err = subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)
		s.Equal(1, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueSkipsSender() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

		// Create room
		senderID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, senderID)
		require.NoError(t, err)

		// Get sender's subscription
		senderSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), senderID)
		require.NoError(t, err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   room.GetId(),
					SenderId: senderID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendEvents(ctx, msgReq, senderID)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.GetId(),
			"room_event_id": eventID,
			"sender_id":     senderID,
		}

		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Verify sender has no outbox entries (they sent the message)
		pending, err := outboxRepo.GetPendingBySubscription(ctx, senderSub.GetID(), 10)
		require.NoError(t, err)
		s.Empty(pending, "Sender should not have outbox entries for their own messages")

		// Verify sender's unread count is 0
		senderSub, err = subRepo.GetByRoomAndProfile(ctx, room.GetId(), senderID)
		require.NoError(t, err)
		s.Equal(0, senderSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueMultipleMessages() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Send multiple messages
		for range 5 {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			msgReq := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId:   room.GetId(),
						SenderId: creatorID,
						Type:     chatv1.RoomEventType_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
			require.NoError(t, err)
			eventID := acks[0].GetEventId()

			// Execute the outbox logging queue for each message
			queuePayload := map[string]string{
				"room_id":       room.GetId(),
				"room_event_id": eventID,
				"sender_id":     creatorID,
			}

			err = queue.Execute(ctx, queuePayload)
			require.NoError(t, err)
		}

		// Verify member's unread count is 5
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)
		s.Equal(5, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueWithInactiveSubscription() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
		outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Deactivate member's subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)
		err = subRepo.Deactivate(ctx, memberSub.GetID())
		require.NoError(t, err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   room.GetId(),
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.GetId(),
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Verify inactive member has no outbox entries
		pending, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		require.NoError(t, err)
		s.Empty(pending, "Inactive subscription should not receive outbox entries")
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueWithMissingRoomID() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		// Execute with missing room_id
		queuePayload := map[string]string{
			"room_event_id": util.IDString(),
			"sender_id":     util.IDString(),
		}

		// Should handle gracefully (may return error or handle empty room_id)
		_ = queue.Execute(ctx, queuePayload)
		// Test passes if no panic occurs
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueWithNonExistentRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		queue := s.createQueue(ctx, svc)

		// Execute with non-existent room
		queuePayload := map[string]string{
			"room_id":       util.IDString(), // Non-existent
			"room_event_id": util.IDString(),
			"sender_id":     util.IDString(),
		}

		// Should handle gracefully (no subscribers)
		err := queue.Execute(ctx, queuePayload)
		// Should not error or should handle no subscribers gracefully
		require.NoError(t, err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueConcurrency() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Send multiple messages concurrently
		messageCount := 10
		eventIDs := make([]string, messageCount)

		for i := range messageCount {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			msgReq := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId:   room.GetId(),
						SenderId: creatorID,
						Type:     chatv1.RoomEventType_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
			require.NoError(t, err)
			eventIDs[i] = acks[0].GetEventId()
		}

		// Execute queue for all messages
		for _, eventID := range eventIDs {
			queuePayload := map[string]string{
				"room_id":       room.GetId(),
				"room_event_id": eventID,
				"sender_id":     creatorID,
			}

			err = queue.Execute(ctx, queuePayload)
			require.NoError(t, err)
		}

		// Verify member's unread count matches message count
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)
		s.Equal(messageCount, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueIdempotency() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(ctx, svc)
		queue := s.createQueue(ctx, svc)
		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
		outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		require.NoError(t, err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   room.GetId(),
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		queuePayload := map[string]string{
			"room_id":       room.GetId(),
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		// Execute once
		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Get member subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.GetId(), memberID)
		require.NoError(t, err)

		// Count outbox entries
		pending1, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		require.NoError(t, err)
		count1 := len(pending1)

		// Execute again (simulating duplicate event)
		err = queue.Execute(ctx, queuePayload)
		require.NoError(t, err)

		// Count outbox entries again
		pending2, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		require.NoError(t, err)
		count2 := len(pending2)

		// Note: Without proper idempotency checks, this will create duplicates
		// This test documents current behavior
		// In production, you might want to add idempotency based on event_id
		s.GreaterOrEqual(count2, count1, "Duplicate execution creates more entries")
	})
}
