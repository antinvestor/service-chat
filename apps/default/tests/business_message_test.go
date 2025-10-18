package tests

import (
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"

	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"
)

type MessageBusinessTestSuite struct {
	BaseTestSuite
}

func TestMessageBusinessTestSuite(t *testing.T) {
	suite.Run(t, new(MessageBusinessTestSuite))
}

func (s *MessageBusinessTestSuite) setupBusinessLayer(svc *frame.Service) (business.MessageBusiness, business.RoomBusiness) {
	roomRepo := repository.NewRoomRepository(svc)
	eventRepo := repository.NewRoomEventRepository(svc)
	subRepo := repository.NewRoomSubscriptionRepository(svc)
	outboxRepo := repository.NewRoomOutboxRepository(svc)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return messageBusiness, roomBusiness
}

func (s *MessageBusinessTestSuite) TestSendMessage() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

		// Create room first
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Hello World",
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
		s.Len(acks, 1)
		s.NotEmpty(acks[0].EventId)
	})
}

func (s *MessageBusinessTestSuite) TestSendMessageToNonExistentRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, _ := s.setupBusinessLayer(svc)

		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Hello",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   util.IDString(), // Non-existent room
					SenderId: util.IDString(),
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, util.IDString())
		s.NoError(err) // Should return acks with errors
		s.Len(acks, 1)
		// Check if ack contains error in metadata
		s.NotNil(acks[0].Metadata)
	})
}

func (s *MessageBusinessTestSuite) TestSendMultipleMessages() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send multiple messages
		messages := []*chatv1.RoomEvent{}
		for i := 0; i < 5; i++ {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			messages = append(messages, &chatv1.RoomEvent{
				RoomId:   room.Id,
				SenderId: creatorID,
				Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
				Payload:  payload,
			})
		}

		msgReq := &chatv1.SendMessageRequest{
			Message: messages,
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
		s.NoError(err)
		s.Len(acks, 5)

		for _, ack := range acks {
			s.NotEmpty(ack.EventId)
		}
	})
}

func (s *MessageBusinessTestSuite) TestGetHistory() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

		// Create room and send messages
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send 10 messages
		for i := 0; i < 10; i++ {
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

			_, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
			s.NoError(err)
		}

		// Get history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.Id,
			Limit:  5,
		}

		events, err := messageBusiness.GetHistory(ctx, historyReq, creatorID)
		s.NoError(err)
		s.Len(events, 5)
	})
}

func (s *MessageBusinessTestSuite) TestGetMessageViaHistory() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

		// Create room and send message
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test Message",
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
		messageID := acks[0].EventId

		// Get the message via history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.Id,
			Limit:  10,
		}

		events, err := messageBusiness.GetHistory(ctx, historyReq, creatorID)
		s.NoError(err)
		s.NotEmpty(events)

		// Find our message
		found := false
		for _, event := range events {
			if event.Id == messageID {
				found = true
				break
			}
		}
		s.True(found, "Message should be in history")
	})
}

func (s *MessageBusinessTestSuite) TestDeleteMessageViaRepository() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)
		eventRepo := repository.NewRoomEventRepository(svc)

		// Create room and send message
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Message to Delete",
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
		messageID := acks[0].EventId

		// Delete the message via repository
		err = eventRepo.Delete(ctx, messageID)
		s.NoError(err)

		// Verify deletion
		_, err = eventRepo.GetByID(ctx, messageID)
		s.Error(err)
	})
}

func (s *MessageBusinessTestSuite) TestMarkMessagesAsRead() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

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

		// Send message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test Message",
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

		// Mark as read by member
		err = messageBusiness.MarkMessagesAsRead(ctx, room.Id, eventID, memberID)
		s.NoError(err)
	})
}

func (s *MessageBusinessTestSuite) TestSendDifferentMessageTypes() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Test different message types
		messageTypes := []chatv1.RoomEventType{
			chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
			chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		}

		for _, msgType := range messageTypes {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"data": "test",
			})

			msgReq := &chatv1.SendMessageRequest{
				Message: []*chatv1.RoomEvent{
					{
						RoomId:   room.Id,
						SenderId: creatorID,
						Type:     msgType,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendMessage(ctx, msgReq, creatorID)
			s.NoError(err)
			s.Len(acks, 1)
			s.NotEmpty(acks[0].EventId)
		}
	})
}
