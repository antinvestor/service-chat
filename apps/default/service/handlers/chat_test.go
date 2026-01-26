package handlers_test

import (
	"context"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service/handlers"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/frame/security"
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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		msgReq := connect.NewRequest(&chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId: roomID,
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
					},
				},
			},
		})

		msgResp, err := chatServer.SendEvent(ctx, msgReq)
		require.NoError(t, err)
		s.Len(msgResp.Msg.GetAck(), 1)
		s.NotEmpty(msgResp.Msg.GetAck()[0].GetEventId())
		s.Len(msgResp.Msg.GetAck()[0].GetEventId(), 1)
	})
}

func (s *ChatServerTestSuite) TestGetHistory() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		for range 5 {
			msgReq := connect.NewRequest(&chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{
					{
						RoomId: roomID,
						Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
						Payload: &chatv1.Payload{
							Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
						},
					},
				},
			})

			_, err = chatServer.SendEvent(ctx, msgReq)
			require.NoError(t, err)
		}

		// Get history
		historyReq := connect.NewRequest(&chatv1.GetHistoryRequest{
			RoomId: roomID,
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		})

		historyResp, err := chatServer.GetHistory(ctx, historyReq)
		require.NoError(t, err)
		s.GreaterOrEqual(len(historyResp.Msg.GetEvents()), 5)
	})
}

func (s *ChatServerTestSuite) TestAddRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		memberContactID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					Member: &commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
					Roles:  []string{"member"},
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
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		memberContactID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					Member: &commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
					Roles:  []string{"member"},
				},
			},
		})

		_, err = chatServer.AddRoomSubscriptions(ctx, addReq)
		require.NoError(t, err)

		// Get subscription ID
		searchReq := connect.NewRequest(&chatv1.SearchRoomSubscriptionsRequest{
			RoomId: roomID,
		})
		searchResp, err := chatServer.SearchRoomSubscriptions(ctx, searchReq)
		require.NoError(t, err)

		var subscriptionID string
		for _, sub := range searchResp.Msg.GetMembers() {
			if sub.GetMember().GetProfileId() == memberID {
				subscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, subscriptionID)

		// Remove member
		removeReq := connect.NewRequest(&chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:         roomID,
			SubscriptionId: []string{subscriptionID},
		})

		_, err = chatServer.RemoveRoomSubscriptions(ctx, removeReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestUpdateSubscriptionRole() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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
		memberContactID := util.IDString()
		addReq := connect.NewRequest(&chatv1.AddRoomSubscriptionsRequest{
			RoomId: roomID,
			Members: []*chatv1.RoomSubscription{
				{
					Member: &commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
					Roles:  []string{"member"},
				},
			},
		})

		_, err = chatServer.AddRoomSubscriptions(ctx, addReq)
		require.NoError(t, err)

		// Get subscription ID
		searchReq := connect.NewRequest(&chatv1.SearchRoomSubscriptionsRequest{
			RoomId: roomID,
		})
		searchResp, err := chatServer.SearchRoomSubscriptions(ctx, searchReq)
		require.NoError(t, err)

		var subscriptionID string
		for _, sub := range searchResp.Msg.GetMembers() {
			if sub.GetMember().GetProfileId() == memberID {
				subscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, subscriptionID)

		// Update role to moderator
		updateReq := connect.NewRequest(&chatv1.UpdateSubscriptionRoleRequest{
			RoomId:         roomID,
			SubscriptionId: subscriptionID,
			Roles:          []string{"moderator"},
		})

		_, err = chatServer.UpdateSubscriptionRole(ctx, updateReq)
		require.NoError(t, err)
	})
}

func (s *ChatServerTestSuite) TestSearchRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

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

// withSystemAuth creates a context with system_internal role for Live API tests.
func (s *ChatServerTestSuite) withSystemAuth(ctx context.Context, profileID string) context.Context {
	claims := &security.AuthenticationClaims{
		TenantID:  util.IDString(),
		AccessID:  util.IDString(),
		ContactID: profileID,
		SessionID: util.IDString(),
		DeviceID:  "test-device",
		Roles:     []string{"system_internal"},
	}
	claims.Subject = profileID
	return claims.ClaimsToContext(ctx)
}

func (s *ChatServerTestSuite) TestLive_TypingIndicator() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

		profileID := util.IDString()
		ctx = s.withSystemAuth(ctx, profileID)

		// Create a room first
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "Typing Test Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		// Send typing indicator via Live
		liveReq := connect.NewRequest(&chatv1.LiveRequest{
			ClientStates: []*chatv1.ClientCommand{
				{
					State: &chatv1.ClientCommand_Typing{
						Typing: &chatv1.TypingEvent{
							RoomId: roomID,
							Typing: true,
						},
					},
				},
			},
		})

		resp, err := chatServer.Live(ctx, liveReq)
		require.NoError(t, err)
		s.NotNil(resp)
	})
}

func (s *ChatServerTestSuite) TestLive_ReadMarker() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

		profileID := util.IDString()
		ctx = s.withSystemAuth(ctx, profileID)

		// Create room and send a message
		createReq := connect.NewRequest(&chatv1.CreateRoomRequest{
			Name:      "ReadMarker Test Room",
			IsPrivate: false,
		})

		createResp, err := chatServer.CreateRoom(ctx, createReq)
		require.NoError(t, err)
		roomID := createResp.Msg.GetRoom().GetId()

		msgReq := connect.NewRequest(&chatv1.SendEventRequest{
			Event: []*chatv1.RoomEvent{
				{
					RoomId:  roomID,
					Type:    chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "hello"}}},
				},
			},
		})

		msgResp, err := chatServer.SendEvent(ctx, msgReq)
		require.NoError(t, err)
		eventID := msgResp.Msg.GetAck()[0].GetEventId()[0]

		// Mark as read via Live
		liveReq := connect.NewRequest(&chatv1.LiveRequest{
			ClientStates: []*chatv1.ClientCommand{
				{
					State: &chatv1.ClientCommand_ReadMarker{
						ReadMarker: &chatv1.ReadMarker{
							RoomId:      &roomID,
							UpToEventId: eventID,
						},
					},
				},
			},
		})

		resp, err := chatServer.Live(ctx, liveReq)
		require.NoError(t, err)
		s.NotNil(resp)
	})
}

func (s *ChatServerTestSuite) TestLive_EmptyClientStates() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

		profileID := util.IDString()
		ctx = s.withSystemAuth(ctx, profileID)

		liveReq := connect.NewRequest(&chatv1.LiveRequest{
			ClientStates: []*chatv1.ClientCommand{},
		})

		_, err := chatServer.Live(ctx, liveReq)
		require.Error(t, err)
	})
}

func (s *ChatServerTestSuite) TestLive_Unauthenticated() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		chatServer := handlers.NewChatServer(ctx, svc, nil, nil, nil)

		liveReq := connect.NewRequest(&chatv1.LiveRequest{
			ClientStates: []*chatv1.ClientCommand{
				{
					State: &chatv1.ClientCommand_Presence{
						Presence: &chatv1.PresenceEvent{
							Status: chatv1.PresenceStatus_PRESENCE_STATUS_ONLINE,
						},
					},
				},
			},
		})

		_, err := chatServer.Live(ctx, liveReq)
		require.Error(t, err)
	})
}
