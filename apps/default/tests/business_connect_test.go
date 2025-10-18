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

type ConnectBusinessTestSuite struct {
	BaseTestSuite
}

func TestConnectBusinessTestSuite(t *testing.T) {
	suite.Run(t, new(ConnectBusinessTestSuite))
}

func (s *ConnectBusinessTestSuite) setupBusinessLayer(svc *frame.Service) (business.ConnectBusiness, business.RoomBusiness, business.MessageBusiness) {
	roomRepo := repository.NewRoomRepository(svc)
	eventRepo := repository.NewRoomEventRepository(svc)
	subRepo := repository.NewRoomSubscriptionRepository(svc)
	outboxRepo := repository.NewRoomOutboxRepository(svc)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	connectBusiness := business.NewConnectBusiness(svc, subRepo, eventRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return connectBusiness, roomBusiness, messageBusiness
}

func (s *ConnectBusinessTestSuite) TestBroadcastEvent() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, _ := s.setupBusinessLayer(svc)

		// Create room with members
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

		// Create a server event
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Broadcast message",
		})

		roomEvent := &chatv1.RoomEvent{
			RoomId:   room.Id,
			SenderId: creatorID,
			Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
			Payload:  payload,
		}

		event := &chatv1.ServerEvent{
			Payload: &chatv1.ServerEvent_Message{
				Message: roomEvent,
			},
		}

		// Broadcast event (should not error even without active connections)
		err = connectBusiness.BroadcastEvent(ctx, room.Id, event)
		s.NoError(err)
	})
}

func (s *ConnectBusinessTestSuite) TestSendPresenceUpdate() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, _ := s.setupBusinessLayer(svc)

		// Create room
		userID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, userID)
		s.NoError(err)

		// Send presence update
		err = connectBusiness.SendPresenceUpdate(ctx, userID, room.Id, chatv1.PresenceStatus_PRESENCE_ONLINE)
		s.NoError(err)

		// Send offline status
		err = connectBusiness.SendPresenceUpdate(ctx, userID, room.Id, chatv1.PresenceStatus_PRESENCE_OFFLINE)
		s.NoError(err)
	})
}

func (s *ConnectBusinessTestSuite) TestSendTypingIndicator() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, _ := s.setupBusinessLayer(svc)

		// Create room
		userID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, userID)
		s.NoError(err)

		// Send typing indicator
		err = connectBusiness.SendTypingIndicator(ctx, userID, room.Id, true)
		s.NoError(err)

		// Send stopped typing
		err = connectBusiness.SendTypingIndicator(ctx, userID, room.Id, false)
		s.NoError(err)
	})
}

func (s *ConnectBusinessTestSuite) TestSendReadReceipt() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, messageBusiness := s.setupBusinessLayer(svc)

		// Create room
		userID := util.IDString()
		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, userID)
		s.NoError(err)

		// Send a message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Test message",
		})

		msgReq := &chatv1.SendMessageRequest{
			Message: []*chatv1.RoomEvent{
				{
					RoomId:   room.Id,
					SenderId: userID,
					Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
					Payload:  payload,
				},
			},
		}

		acks, err := messageBusiness.SendMessage(ctx, msgReq, userID)
		s.NoError(err)
		eventID := acks[0].EventId

		// Send read receipt
		err = connectBusiness.SendReadReceipt(ctx, userID, room.Id, eventID)
		s.NoError(err)
	})
}

func (s *ConnectBusinessTestSuite) TestSendReadReceiptAccessDenied() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, messageBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		nonMemberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Private Room",
			IsPrivate: true,
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

		// Try to send read receipt as non-member
		err = connectBusiness.SendReadReceipt(ctx, nonMemberID, room.Id, eventID)
		s.Error(err)
	})
}

func (s *ConnectBusinessTestSuite) TestBroadcastEventValidation() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, _, _ := s.setupBusinessLayer(svc)

		// Try to broadcast with empty room ID
		roomEvent := &chatv1.RoomEvent{
			Type: chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
		}

		event := &chatv1.ServerEvent{
			Payload: &chatv1.ServerEvent_Message{
				Message: roomEvent,
			},
		}

		err := connectBusiness.BroadcastEvent(ctx, "", event)
		s.Error(err)
	})
}

func (s *ConnectBusinessTestSuite) TestPresenceUpdateValidation() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, _, _ := s.setupBusinessLayer(svc)

		// Try with empty profile ID
		err := connectBusiness.SendPresenceUpdate(ctx, "", util.IDString(), chatv1.PresenceStatus_PRESENCE_ONLINE)
		s.Error(err)

		// Try with empty room ID
		err = connectBusiness.SendPresenceUpdate(ctx, util.IDString(), "", chatv1.PresenceStatus_PRESENCE_ONLINE)
		s.Error(err)
	})
}

func (s *ConnectBusinessTestSuite) TestTypingIndicatorValidation() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, _, _ := s.setupBusinessLayer(svc)

		// Try with empty profile ID
		err := connectBusiness.SendTypingIndicator(ctx, "", util.IDString(), true)
		s.Error(err)

		// Try with empty room ID
		err = connectBusiness.SendTypingIndicator(ctx, util.IDString(), "", true)
		s.Error(err)
	})
}

func (s *ConnectBusinessTestSuite) TestMultiplePresenceUpdates() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		connectBusiness, roomBusiness, _ := s.setupBusinessLayer(svc)

		// Create room with multiple members
		creatorID := util.IDString()
		members := []string{util.IDString(), util.IDString(), util.IDString()}

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   members,
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Send presence updates for all members
		allUsers := append([]string{creatorID}, members...)
		for _, userID := range allUsers {
			err = connectBusiness.SendPresenceUpdate(ctx, userID, room.Id, chatv1.PresenceStatus_PRESENCE_ONLINE)
			s.NoError(err)
		}

		// Send typing indicators
		for _, userID := range allUsers {
			err = connectBusiness.SendTypingIndicator(ctx, userID, room.Id, true)
			s.NoError(err)
		}
	})
}
