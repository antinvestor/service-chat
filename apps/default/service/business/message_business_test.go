package business_test

import (
	"context"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MessageBusinessTestSuite struct {
	tests.BaseTestSuite
}

func TestMessageBusinessTestSuite(t *testing.T) {
	suite.Run(t, new(MessageBusinessTestSuite))
}

func (s *MessageBusinessTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) (business.MessageBusiness, business.RoomBusiness) {
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness, nil)

	return messageBusiness, roomBusiness
}

func (s *MessageBusinessTestSuite) TestSendMessage() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room first
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Send message
		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId: room.GetId(),
					Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		}

		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Len(acks, 1)
		s.NotEmpty(acks[0].GetEventId())
	})
}

func (s *MessageBusinessTestSuite) TestSendMessageToNonExistentRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, _ := s.setupBusinessLayer(ctx, svc)

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId: util.IDString(), // Non-existent room
					Source: &commonv1.ContactLink{ProfileId: util.IDString(), ContactId: util.IDString()},
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		}

		senderID := util.IDString()
		senderContactID := util.IDString()
		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: senderID, ContactId: senderContactID},
		)
		require.NoError(t, err) // Should return acks with errors
		s.Len(acks, 1)
		// Check if ack contains error in metadata
		s.NotNil(acks[0].GetMetadata())
	})
}

func (s *MessageBusinessTestSuite) TestSendMultipleMessages() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Send multiple messages
		var messages []*chatv1.RoomEvent
		for range 5 {
			messages = append(messages, &chatv1.RoomEvent{
				RoomId:  room.GetId(),
				Source:  &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
				Type:    chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
				Payload: &chatv1.Payload{Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}}},
			})
		}

		msgReq := &chatv1.SendEventRequest{
			Event: messages,
		}

		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Len(acks, 5)

		for _, ack := range acks {
			s.NotEmpty(ack.GetEventId())
		}
	})
}

func (s *MessageBusinessTestSuite) TestGetHistory() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room and send messages
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Send 10 messages
		for range 10 {
			msgReq := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId: room.GetId(),
						Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
						Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
						Payload: &chatv1.Payload{
							Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
						},
					},
				},
			}

			_, sendErr := messageBusiness.SendEvents(
				ctx,
				msgReq,
				&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			)
			require.NoError(t, sendErr)
		}

		// Get history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 5, Page: ""},
		}

		events, err := messageBusiness.GetHistory(
			ctx,
			historyReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Len(events, 5)
	})
}

func (s *MessageBusinessTestSuite) TestGetMessageViaHistory() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room and send message
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId: room.GetId(),
					Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		}

		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		messageID := acks[0].GetEventId()

		// Get the message via history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		}

		events, err := messageBusiness.GetHistory(
			ctx,
			historyReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.NotEmpty(events)

		// Find our message
		found := false
		for _, event := range events {
			if event.GetId() == messageID {
				found = true
				break
			}
		}
		s.True(found, "Message should be in history")
	})
}

func (s *MessageBusinessTestSuite) TestDeleteMessageViaRepository() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		workMan := svc.WorkManager()
		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

		eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)

		// Create room and send message
		creatorID := util.IDString()
		creatorContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					Id:     util.IDString(),
					RoomId: room.GetId(),
					Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		}

		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		messageID := acks[0].GetEventId()

		_, err = eventRepo.GetByID(ctx, messageID)
		require.NoError(t, err)

		// Delete the message via repository
		err = eventRepo.Delete(ctx, messageID)
		require.NoError(t, err)

		// Verify deletion
		_, err = eventRepo.GetByID(ctx, messageID)
		require.Error(t, err)
		require.True(t, data.ErrorIsNoRows(err))
	})
}

func (s *MessageBusinessTestSuite) TestMarkMessagesAsRead() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room with member
		creatorID := util.IDString()
		creatorContactID := util.IDString()

		memberID := util.IDString()
		memberContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{{ProfileId: memberID, ContactId: memberContactID}},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Send message
		msgReq := &chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId: room.GetId(),
					Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		}

		acks, err := messageBusiness.SendEvents(
			ctx,
			msgReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		eventID := acks[0].GetEventId()

		// Mark as read by member
		err = messageBusiness.MarkMessagesAsRead(
			ctx,
			room.GetId(),
			eventID,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
		)
		require.NoError(t, err)
	})
}

func (s *MessageBusinessTestSuite) TestSendDifferentMessageTypes() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		messageBusiness, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Test different message types
		messageTypes := []chatv1.RoomEventType{
			chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
		}

		for _, msgType := range messageTypes {
			msgReq := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId: room.GetId(),
						Source: &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
						Type:   msgType,
						Payload: &chatv1.Payload{
							Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
						},
					},
				},
			}

			acks, sendErr := messageBusiness.SendEvents(
				ctx,
				msgReq,
				&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			)
			require.NoError(t, sendErr)
			s.Len(acks, 1)
			s.NotEmpty(acks[0].GetEventId())
		}
	})
}
