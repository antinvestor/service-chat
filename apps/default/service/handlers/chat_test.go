package handlers_test

import (
	"testing"

	"connectrpc.com/connect"
	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/handlers"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"
)

type ChatServerTestSuite struct {
	tests.BaseTestSuite
}

func TestChatServerTestSuite(t *testing.T) {
	suite.Run(t, new(ChatServerTestSuite))
}

func (s *ChatServerTestSuite) TestCreateRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		req := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:        "Test Room",
			Description: "A test room",
			IsPrivate:   false,
			Metadata:    &structpb.Struct{Fields: map[string]*structpb.Value{}},
		})

		resp, err := chatServer.CreateRoom(ctx, req)
		require.NoError(t, err)
		s.NotNil(resp)
		s.NotEmpty(resp.Msg.GetRoom().GetId())
		s.Equal("Test Room", resp.Msg.GetRoom().GetName())
	})
}

func (s *ChatServerTestSuite) TestCreateRoomUnauthenticated() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		req := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		})

		_, err := chatServer.CreateRoom(ctx, req)
		require.Error(t, err)
	})
}

func (s *ChatServerTestSuite) TestUpdateRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room first
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Original Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Update the room
		updateReq := connect.NewRequest(&chatv1.UpdateRoomRequest{
			RoomId: roomID,
			Name:   "Updated Room",
			Topic:  "Updated topic",
		})

		updateResp, err := chatServer.UpdateRoom(ctx, updateReq)
		require.NoError(t, err)
		s.Equal("Updated Room", updateResp.Msg.GetRoom().GetName())
	})
}

func (s *ChatServerTestSuite) TestDeleteRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Room to Delete",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Delete the room
		deleteReq := connect.NewRequest(&chatv1.DeleteRoomRequest{
			RoomId: roomID,
		})

		_, err = chatServer.DeleteRoom(ctx, deleteReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestSendEvent() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Message Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Send message
		payload, _ := structpb.NewStruct(map[string]interface{}{
			"text": "Hello World",
		})

		msgReq := connect.NewRequest(&chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:   roomID,
					SenderId: profileID,
					Type:     chatv1.RoomEventType_TEXT,
					Payload:  payload,
				},
			},
		})

		msgResp, err := chatServer.SendEvent(ctx, msgReq)
		require.NoError(t, err)
		s.Len(msgResp.Msg.GetAck(), 1)
		s.NotEmpty(msgResp.Msg.GetAck()[0].GetEventId())
	})
}

func (s *ChatServerTestSuite) TestGetHistory() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "History Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Send messages
		for i := 0; i < 5; i++ {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": "Message",
			})

			msgReq := connect.NewRequest(&chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId:   roomID,
						SenderId: profileID,
						Type:     chatv1.RoomEventType_TEXT,
						Payload:  payload,
					},
				},
			})

			_, err = chatServer.SendEvent(ctx, msgReq)
			require.NoError(t, err)
		}

		// Get history
		historyReq := connect.NewRequest(&chatv1.GetHistoryRequest{
			RoomId: roomID,
			Limit:  10,
		})

		historyResp, err := chatServer.GetHistory(ctx, historyReq)
		require.NoError(t, err)
		s.GreaterOrEqual(len(historyResp.Msg.GetEvents()), 5)
	})
}

func (s *ChatServerTestSuite) TestAddRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Subscription Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Add member
		memberID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					ProfileId: memberID,
					Roles:     []string{"member"},
				},
			},
		})

		_, err = chatServer.AddRoomSubscriptions(ctx, addReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestRemoveRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Removal Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Add member
		memberID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					ProfileId: memberID,
					Roles:     []string{"member"},
				},
			},
		})

		_, err = chatServer.AddRoomSubscriptions(ctx, addReq)
		require.NoError(t, err)

		// Remove member
		removeReq := connect.NewRequest(&chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:     roomID,
			ProfileIds: []string{memberID},
		})

		_, err = chatServer.RemoveRoomSubscriptions(ctx, removeReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestUpdateSubscriptionRole() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Role Update Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Add member
		memberID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					ProfileId: memberID,
					Roles:     []string{"member"},
				},
			},
		})

		_, err = chatServer.AddRoomSubscriptions(ctx, addReq)
		require.NoError(t, err)

		// Update role to moderator
		updateReq := connect.NewRequest(&chatv1.UpdateSubscriptionRoleRequest{
			RoomId:    roomID,
			ProfileId: memberID,
			Roles:     []string{"moderator"},
		})

		_, err = chatServer.UpdateSubscriptionRole(ctx, updateReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestSearchRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Search Subscriptions Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Search subscriptions
		searchReq := connect.NewRequest(&chatv1.SearchRoomSubscriptionsRequest{
			RoomId: roomID,
		})

		searchResp, err := chatServer.SearchRoomSubscriptions(ctx, searchReq)
		require.NoError(t, err)
		s.GreaterOrEqual(len(searchResp.Msg.GetMembers()), 1) // At least the creator
	})
}

func (s *ChatServerTestSuite) TestUpdateClientState() {
	s.T().Skip("Requires ProfileCli mock - skipping for now")
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil)

		profileID := util.IDString()
		ctx = s.WithAuthClaims(ctx, profileID)

		// Create room
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Client State Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Update typing state
		stateReq := connect.NewRequest(&chatv1.UpdateClientStateRequest{
			RoomId:    roomID,
			ProfileId: profileID,
			ClientStates: []*chatv1.ClientState{
				{
					State: &chatv1.ClientState_Typing{
						Typing: &chatv1.TypingEvent{
							RoomId:    roomID,
							ProfileId: profileID,
							Typing:    true,
						},
					},
				},
			},
		})

		_, err = chatServer.UpdateClientState(ctx, stateReq)
		require.NoError(t, err)
	})
}
