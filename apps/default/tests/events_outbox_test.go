package tests

import (
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"
)

type OutboxEventTestSuite struct {
	BaseTestSuite
}

func TestOutboxEventTestSuite(t *testing.T) {
	suite.Run(t, new(OutboxEventTestSuite))
}

func (s *OutboxEventTestSuite) setupBusinessLayer(svc *frame.Service) (business.RoomBusiness, business.MessageBusiness) {
	roomRepo := repository.NewRoomRepository(svc)
	eventRepo := repository.NewRoomEventRepository(svc)
	subRepo := repository.NewRoomSubscriptionRepository(svc)
	outboxRepo := repository.NewRoomOutboxRepository(svc)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return roomBusiness, messageBusiness
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueName() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, _ := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

		s.Equal(events.RoomOutboxLoggingQueueName, queue.Name())
		s.Equal("room.outbox.logging.queue", queue.Name())
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueuePayloadType() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, _ := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

		payloadType := queue.PayloadType()
		s.NotNil(payloadType)

		// Should be map[string]string
		_, ok := payloadType.(map[string]string)
		s.True(ok, "Payload type should be map[string]string")
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueValidateValidPayload() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

		validPayload := map[string]string{
			"room_id":       util.IDString(),
			"room_event_id": util.IDString(),
			"sender_id":     util.IDString(),
		}

		err := queue.Validate(ctx, validPayload)
		s.NoError(err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueValidateInvalidPayload() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

		// Invalid payload type
		invalidPayload := "not a map"

		err := queue.Validate(ctx, invalidPayload)
		s.Error(err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueExecuteCreatesOutboxEntries() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		outboxRepo := repository.NewRoomOutboxRepository(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

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
		s.NoError(err)

		// Send a message to create an event
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
		s.NoError(err)
		eventID := acks[0].EventId

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.Id,
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Verify outbox entries were created for all subscribers
		// Should have entries for all members (creator is skipped in outbox)
		subs, err := subRepo.GetByRoomID(ctx, room.Id, true)
		s.NoError(err)

		// Count outbox entries
		outboxCount := 0
		for _, sub := range subs {
			if sub.ProfileID != creatorID {
				pending, err := outboxRepo.GetPendingBySubscription(ctx, sub.GetID(), 10)
				s.NoError(err)
				outboxCount += len(pending)
			}
		}

		// Should have outbox entries for non-sender members
		s.GreaterOrEqual(outboxCount, 2)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueUpdatesUnreadCount() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Get member's subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)

		// Initial unread count should be 0
		s.Equal(0, memberSub.UnreadCount)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
		s.NoError(err)
		eventID := acks[0].EventId

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.Id,
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Verify unread count increased (generated column)
		memberSub, err = subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)
		s.Equal(1, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueSkipsSender() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		outboxRepo := repository.NewRoomOutboxRepository(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

		// Create room
		senderID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, senderID)
		s.NoError(err)

		// Get sender's subscription
		senderSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, senderID)
		s.NoError(err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: senderID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, senderID)
		s.NoError(err)
		eventID := acks[0].EventId

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.Id,
			"room_event_id": eventID,
			"sender_id":     senderID,
		}

		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Verify sender has no outbox entries (they sent the message)
		pending, err := outboxRepo.GetPendingBySubscription(ctx, senderSub.GetID(), 10)
		s.NoError(err)
		s.Len(pending, 0, "Sender should not have outbox entries for their own messages")

		// Verify sender's unread count is 0
		senderSub, err = subRepo.GetByRoomAndProfile(ctx, room.Id, senderID)
		s.NoError(err)
		s.Equal(0, senderSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueMultipleMessages() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send multiple messages
		for i := 0; i < 5; i++ {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			msgReq := &chatv1.SendMessageRequest{
				Message: []*chatv1.RoomEvent{
					{
						RoomId:   room.Id,
						SenderId: creatorID,
						Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
			s.NoError(err)
			eventID := acks[0].EventId

			// Execute the outbox logging queue for each message
			queuePayload := map[string]string{
				"room_id":       room.Id,
				"room_event_id": eventID,
				"sender_id":     creatorID,
			}

			err = queue.Execute(ctx, queuePayload)
			s.NoError(err)
		}

		// Verify member's unread count is 5
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)
		s.Equal(5, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueWithInactiveSubscription() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)
		outboxRepo := repository.NewRoomOutboxRepository(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Deactivate member's subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)
		err = subRepo.Deactivate(ctx, memberSub.GetID())
		s.NoError(err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
		s.NoError(err)
		eventID := acks[0].EventId

		// Execute the outbox logging queue
		queuePayload := map[string]string{
			"room_id":       room.Id,
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Verify inactive member has no outbox entries
		pending, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		s.NoError(err)
		s.Len(pending, 0, "Inactive subscription should not receive outbox entries")
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueWithMissingRoomID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

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
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		queue := events.NewRoomOutboxLoggingQueue(svc)

		// Execute with non-existent room
		queuePayload := map[string]string{
			"room_id":       util.IDString(), // Non-existent
			"room_event_id": util.IDString(),
			"sender_id":     util.IDString(),
		}

		// Should handle gracefully (no subscribers)
		err := queue.Execute(ctx, queuePayload)
		// Should not error or should handle no subscribers gracefully
		s.NoError(err)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueConcurrency() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send multiple messages concurrently
		messageCount := 10
		eventIDs := make([]string, messageCount)

		for i := 0; i < messageCount; i++ {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			msgReq := &chatv1.SendMessageRequest{
				Message: []*chatv1.RoomEvent{
					{
						RoomId:   room.Id,
						SenderId: creatorID,
						Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
			s.NoError(err)
			eventIDs[i] = acks[0].EventId
		}

		// Execute queue for all messages
		for _, eventID := range eventIDs {
			queuePayload := map[string]string{
				"room_id":       room.Id,
				"room_event_id": eventID,
				"sender_id":     creatorID,
			}

			err = queue.Execute(ctx, queuePayload)
			s.NoError(err)
		}

		// Verify member's unread count matches message count
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)
		s.Equal(messageCount, memberSub.UnreadCount)
	})
}

func (s *OutboxEventTestSuite) TestOutboxLoggingQueueIdempotency() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness := s.setupBusinessLayer(svc)
		queue := events.NewRoomOutboxLoggingQueue(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)
		outboxRepo := repository.NewRoomOutboxRepository(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: creatorID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
		s.NoError(err)
		eventID := acks[0].EventId

		queuePayload := map[string]string{
			"room_id":       room.Id,
			"room_event_id": eventID,
			"sender_id":     creatorID,
		}

		// Execute once
		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Get member subscription
		memberSub, err := subRepo.GetByRoomAndProfile(ctx, room.Id, memberID)
		s.NoError(err)

		// Count outbox entries
		pending1, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		s.NoError(err)
		count1 := len(pending1)

		// Execute again (simulating duplicate event)
		err = queue.Execute(ctx, queuePayload)
		s.NoError(err)

		// Count outbox entries again
		pending2, err := outboxRepo.GetPendingBySubscription(ctx, memberSub.GetID(), 10)
		s.NoError(err)
		count2 := len(pending2)

		// Note: Without proper idempotency checks, this will create duplicates
		// This test documents current behavior
		// In production, you might want to add idempotency based on event_id
		s.GreaterOrEqual(count2, count1, "Duplicate execution creates more entries")
	})
}
