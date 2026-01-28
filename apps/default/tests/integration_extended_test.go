package tests_test

import (
	"sync"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ExtendedIntegrationTestSuite struct {
	tests.BaseTestSuite
}

func TestExtendedIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ExtendedIntegrationTestSuite))
}

func (s *ExtendedIntegrationTestSuite) setupBusiness(
	t *testing.T, dep *definition.DependencyOption,
) (business.RoomBusiness, business.MessageBusiness, business.SubscriptionService) {
	ctx, svc := s.CreateService(t, dep)
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
	proposalRepo := repository.NewProposalRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(
		svc, roomRepo, eventRepo, subRepo, proposalRepo,
		subscriptionSvc, messageBusiness, nil, nil,
	)

	return roomBusiness, messageBusiness, subscriptionSvc
}

func (s *ExtendedIntegrationTestSuite) makeContactLink() *commonv1.ContactLink {
	return &commonv1.ContactLink{
		ProfileId: util.IDString(),
		ContactId: util.IDString(),
	}
}

// TestConcurrentRoomCreation verifies that creating rooms concurrently
// does not cause data races or database constraint violations.
func (s *ExtendedIntegrationTestSuite) TestConcurrentRoomCreation() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		const numRooms = 10
		creator := s.makeContactLink()

		var wg sync.WaitGroup
		rooms := make([]*chatv1.Room, numRooms)
		errs := make([]error, numRooms)

		for i := range numRooms {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &chatv1.CreateRoomRequest{
					Name:      "Concurrent Room " + util.IDString(),
					IsPrivate: false,
				}
				rooms[idx], errs[idx] = roomBusiness.CreateRoom(ctx, req, creator)
			}(i)
		}

		wg.Wait()

		for i := range numRooms {
			require.NoError(t, errs[i], "room %d creation failed", i)
			require.NotNil(t, rooms[i], "room %d is nil", i)
		}

		// All rooms should have unique IDs
		ids := make(map[string]bool)
		for _, room := range rooms {
			require.False(t, ids[room.GetId()], "duplicate room ID: %s", room.GetId())
			ids[room.GetId()] = true
		}
	})
}

// TestConcurrentMessageSending verifies that sending messages concurrently
// to the same room works correctly.
func (s *ExtendedIntegrationTestSuite) TestConcurrentMessageSending() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, messageBusiness, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()
		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name: "Concurrent Message Room",
		}, creator)
		require.NoError(t, err)

		const numMessages = 20
		var wg sync.WaitGroup
		errs := make([]error, numMessages)

		for i := range numMessages {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := &chatv1.SendEventRequest{
					Event: []*chatv1.RoomEvent{{
						RoomId: room.GetId(),
						Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
						Payload: &chatv1.Payload{
							Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{
								Body: "concurrent message " + util.IDString(),
							}},
						},
					}},
				}
				_, errs[idx] = messageBusiness.SendEvents(ctx, req, creator)
			}(i)
		}

		wg.Wait()

		for i := range numMessages {
			require.NoError(t, errs[i], "message %d send failed", i)
		}

		// Verify all messages are in history
		events, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 50, Page: ""},
		}, creator)
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), numMessages)
	})
}

// TestLargeMessageHistoryWithLimit verifies that GetHistory respects
// the limit parameter and returns the correct number of events.
func (s *ExtendedIntegrationTestSuite) TestLargeMessageHistoryWithLimit() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, messageBusiness, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()
		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name: "Limit Test Room",
		}, creator)
		require.NoError(t, err)

		// Send 25 messages
		const totalMessages = 25
		for range totalMessages {
			req := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{{
					RoomId: room.GetId(),
					Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: &chatv1.Payload{
						Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "paginated message"}},
					},
				}},
			}
			_, sendErr := messageBusiness.SendEvents(ctx, req, creator)
			require.NoError(t, sendErr)
		}

		// Request with limit 10 should return exactly 10
		events10, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		}, creator)
		require.NoError(t, err)
		s.Len(events10, 10)

		// Request with limit 50 should return all messages + system events
		eventsAll, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 50, Page: ""},
		}, creator)
		require.NoError(t, err)
		s.GreaterOrEqual(len(eventsAll), totalMessages,
			"should retrieve at least %d messages (plus system events)", totalMessages)

		// Request with limit 1 should return exactly 1
		events1, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 1, Page: ""},
		}, creator)
		require.NoError(t, err)
		s.Len(events1, 1)

		// Events should be ordered by ID ascending
		for i := 1; i < len(eventsAll); i++ {
			s.Less(eventsAll[i-1].GetId(), eventsAll[i].GetId(),
				"events should be ordered by ID ascending")
		}
	})
}

// TestMessageTypeVariations tests sending different message payload types.
func (s *ExtendedIntegrationTestSuite) TestMessageTypeVariations() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, messageBusiness, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()
		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name: "Type Test Room",
		}, creator)
		require.NoError(t, err)

		payloads := []*chatv1.Payload{
			// Text message
			{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_TEXT,
				Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "hello world"}},
			},
			// Attachment
			{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_ATTACHMENT,
				Data: &chatv1.Payload_Attachment{Attachment: &chatv1.AttachmentContent{
					AttachmentId: util.IDString(),
					Filename:     "image.png",
					MimeType:     "image/png",
				}},
			},
			// Reaction
			{
				Type: chatv1.PayloadType_PAYLOAD_TYPE_REACTION,
				Data: &chatv1.Payload_Reaction{Reaction: &chatv1.ReactionContent{
					Reaction: "👍",
				}},
			},
		}

		for _, payload := range payloads {
			req := &chatv1.SendEventRequest{
				Event: []*chatv1.RoomEvent{{
					RoomId:  room.GetId(),
					Type:    chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
					Payload: payload,
				}},
			}
			acks, sendErr := messageBusiness.SendEvents(ctx, req, creator)
			require.NoError(t, sendErr)
			s.Len(acks, 1, "expected 1 ack for payload type %s", payload.GetType())
		}

		// Verify all messages stored
		events, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		}, creator)
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), len(payloads))
	})
}

// TestRoomSearch verifies room search functionality.
func (s *ExtendedIntegrationTestSuite) TestRoomSearch() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()

		// Create rooms with distinctive names
		names := []string{"Alpha Finance Group", "Beta Trading Hub", "Gamma Support Channel"}
		for _, name := range names {
			_, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
				Name: name,
			}, creator)
			require.NoError(t, err)
		}

		// Search for "Alpha"
		results, err := roomBusiness.SearchRooms(ctx, &chatv1.SearchRoomsRequest{
			Query: "Alpha",
		}, creator)
		require.NoError(t, err)
		s.GreaterOrEqual(len(results), 1, "should find at least 1 room matching 'Alpha'")

		found := false
		for _, r := range results {
			if r.GetName() == "Alpha Finance Group" {
				found = true
				break
			}
		}
		s.True(found, "should find 'Alpha Finance Group' in search results")
	})
}

// TestDuplicateMemberAddition verifies that adding an already-existing member
// is handled gracefully.
func (s *ExtendedIntegrationTestSuite) TestDuplicateMemberAddition() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, subscriptionSvc := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()
		member := s.makeContactLink()

		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name:    "Duplicate Member Test",
			Members: []*commonv1.ContactLink{member},
		}, creator)
		require.NoError(t, err)

		// Verify member has access
		accessMap, err := subscriptionSvc.HasAccess(ctx, member, room.GetId())
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// Try adding the same member again - should not error
		err = roomBusiness.AddRoomSubscriptions(ctx, &chatv1.AddRoomSubscriptionsRequest{
			RoomId: room.GetId(),
			Members: []*chatv1.RoomSubscription{
				{Member: member, Roles: []string{"member"}},
			},
		}, creator)
		// This may succeed or return a partial error - either way, the member
		// should still have access
		_ = err

		accessMap, err = subscriptionSvc.HasAccess(ctx, member, room.GetId())
		require.NoError(t, err)
		s.NotEmpty(accessMap)
	})
}

// TestSearchAllSubscriptionsInRoom verifies that subscription search
// returns all members in a room.
func (s *ExtendedIntegrationTestSuite) TestSearchAllSubscriptionsInRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()

		// Create room with many members
		const numMembers = 15
		members := make([]*commonv1.ContactLink, numMembers)
		for i := range numMembers {
			members[i] = s.makeContactLink()
		}

		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name:    "Pagination Subscription Test",
			Members: members,
		}, creator)
		require.NoError(t, err)

		// Search all subscriptions
		subs, err := roomBusiness.SearchRoomSubscriptions(ctx, &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
		}, creator)
		require.NoError(t, err)

		// Should have all members plus the creator
		s.GreaterOrEqual(len(subs), numMembers+1)
	})
}

// TestNewRoomHistory verifies that a newly created room only contains
// system-generated moderation events (e.g. "Room created"), not user messages.
func (s *ExtendedIntegrationTestSuite) TestNewRoomHistory() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, messageBusiness, _ := s.setupBusiness(t, dep)
		ctx := t.Context()

		creator := s.makeContactLink()
		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name: "New Room",
		}, creator)
		require.NoError(t, err)

		events, err := messageBusiness.GetHistory(ctx, &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		}, creator)
		require.NoError(t, err)

		// Room creation generates system events (e.g. "Room created" moderation event),
		// so the history is not empty. Verify all events are system events.
		for _, evt := range events {
			s.Equal(chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT, evt.GetType(),
				"new room should only have system events, got type %s", evt.GetType())
		}
	})
}

// TestMultipleMemberRoleEscalation tests that role changes are persisted
// and enforced correctly across multiple updates.
func (s *ExtendedIntegrationTestSuite) TestMultipleMemberRoleEscalation() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, subscriptionSvc := s.setupBusiness(t, dep)
		ctx := t.Context()

		owner := s.makeContactLink()
		user := s.makeContactLink()

		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name:    "Role Escalation Test",
			Members: []*commonv1.ContactLink{user},
		}, owner)
		require.NoError(t, err)

		// Find the user's subscription ID
		subs, err := roomBusiness.SearchRoomSubscriptions(ctx, &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
		}, owner)
		require.NoError(t, err)

		var userSubID string
		for _, sub := range subs {
			if sub.GetMember().GetProfileId() == user.GetProfileId() {
				userSubID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, userSubID)

		// Escalate member -> admin
		err = roomBusiness.UpdateSubscriptionRole(ctx, &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:         room.GetId(),
			SubscriptionId: userSubID,
			Roles:          []string{"admin"},
		}, owner)
		require.NoError(t, err)

		// Verify admin role
		hasAdmin, err := subscriptionSvc.HasRole(ctx, user, room.GetId(), business.RoleAdminLevel)
		require.NoError(t, err)
		s.NotNil(hasAdmin, "user should have admin role after escalation")

		// Downgrade admin -> member
		err = roomBusiness.UpdateSubscriptionRole(ctx, &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:         room.GetId(),
			SubscriptionId: userSubID,
			Roles:          []string{"member"},
		}, owner)
		require.NoError(t, err)

		// Verify back to member level
		hasMember, err := subscriptionSvc.HasRole(ctx, user, room.GetId(), business.RoleMemberLevel)
		require.NoError(t, err)
		s.NotNil(hasMember, "user should have member role after downgrade")
	})
}

// TestRoomDeletionCleansSubscriptions verifies that deleting a room
// makes all subscriptions inaccessible.
func (s *ExtendedIntegrationTestSuite) TestRoomDeletionCleansSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		roomBusiness, _, subscriptionSvc := s.setupBusiness(t, dep)
		ctx := t.Context()

		owner := s.makeContactLink()
		member := s.makeContactLink()

		room, err := roomBusiness.CreateRoom(ctx, &chatv1.CreateRoomRequest{
			Name:    "Deletion Cleanup Test",
			Members: []*commonv1.ContactLink{member},
		}, owner)
		require.NoError(t, err)

		// Verify access before deletion
		accessMap, err := subscriptionSvc.HasAccess(ctx, member, room.GetId())
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// Delete room
		err = roomBusiness.DeleteRoom(ctx, &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}, owner)
		require.NoError(t, err)

		// Verify room is gone
		_, err = roomBusiness.GetRoom(ctx, room.GetId(), owner)
		require.Error(t, err)

		// Verify subscriptions are no longer accessible
		accessMap, err = subscriptionSvc.HasAccess(ctx, member, room.GetId())
		if err == nil {
			s.Empty(accessMap, "subscriptions should be cleaned up after room deletion")
		}
		// If error is returned, that also confirms access is denied
	})
}
