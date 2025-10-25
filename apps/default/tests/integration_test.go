package tests

import (
	"context"
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/types/known/structpb"
)

type IntegrationTestSuite struct {
	BaseTestSuite
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) (business.RoomBusiness, business.MessageBusiness, business.SubscriptionService) {

	workMan := svc.WorkManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(dbPool, workMan)
	outboxRepo := repository.NewRoomOutboxRepository(dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return roomBusiness, messageBusiness, subscriptionSvc
}

func (s *IntegrationTestSuite) TestCompleteRoomLifecycle() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness, subscriptionSvc := s.setupBusinessLayer(ctx, svc)

		// 1. Create room
		creatorID := util.IDString()
		member1ID := util.IDString()
		member2ID := util.IDString()

		createReq := &chatv1.CreateRoomRequest{
			Name:        "Integration Test Room",
			Description: "Testing complete workflow",
			IsPrivate:   false,
			Members:     []string{member1ID, member2ID},
		}

		room, err := roomBusiness.CreateRoom(ctx, createReq, creatorID)
		s.NoError(err)
		s.NotNil(room)

		// 2. Verify all members have access
		hasAccess, err := subscriptionSvc.HasAccess(ctx, creatorID, room.GetId())
		s.NoError(err)
		s.True(hasAccess)

		hasAccess, err = subscriptionSvc.HasAccess(ctx, member1ID, room.GetId())
		s.NoError(err)
		s.True(hasAccess)

		// 3. Send messages
		for range 5 {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(20),
			})

			msgReq := &chatv1.SendEventRequest{
				Message: []*chatv1.RoomEvent{
					{
						RoomId:   room.GetId(),
						SenderId: creatorID,
						Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendEvents(ctx, msgReq, creatorID)
			s.NoError(err)
			s.Len(acks, 1)
		}

		// 4. Get history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Limit:  10,
		}

		events, err := messageBusiness.GetHistory(ctx, historyReq, member1ID)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 5)

		// 5. Update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated Room Name",
			Topic:  "Updated Description",
		}

		updated, err := roomBusiness.UpdateRoom(ctx, updateReq, creatorID)
		s.NoError(err)
		s.Equal("Updated Room Name", updated.GetName())

		// 6. Add new member
		newMemberID := util.IDString()
		addReq := &chatv1.AddRoomSubscriptionsRequest{
			RoomId: room.GetId(),
			Members: []*chatv1.RoomSubscription{
				{
					ProfileId: newMemberID,
					Roles:     []string{"member"},
				},
			},
		}

		err = roomBusiness.AddRoomSubscriptions(ctx, addReq, creatorID)
		s.NoError(err)

		// 7. Verify new member has access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, newMemberID, room.GetId())
		s.NoError(err)
		s.True(hasAccess)

		// 8. Remove member
		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:     room.GetId(),
			ProfileIds: []string{member2ID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(ctx, removeReq, creatorID)
		s.NoError(err)

		// 9. Verify removed member no longer has access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, member2ID, room.GetId())
		s.NoError(err)
		s.False(hasAccess)

		// 10. Delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, creatorID)
		s.NoError(err)

		// 11. Verify room is deleted
		_, err = roomBusiness.GetRoom(ctx, room.GetId(), creatorID)
		s.Error(err)
	})
}

func (s *IntegrationTestSuite) TestMultiRoomMessaging() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness, _ := s.setupBusinessLayer(ctx, svc)

		userID := util.IDString()

		// Create multiple rooms
		rooms := []*chatv1.Room{}
		for range 3 {
			createReq := &chatv1.CreateRoomRequest{
				Name:      util.RandomString(10),
				IsPrivate: false,
			}

			room, err := roomBusiness.CreateRoom(ctx, createReq, userID)
			s.NoError(err)
			rooms = append(rooms, room)
		}

		// Send messages to each room
		for _, room := range rooms {
			for range 3 {
				payload, _ := structpb.NewStruct(map[string]interface{}{
					"text": util.RandomString(15),
				})

				msgReq := &chatv1.SendEventRequest{
					Message: []*chatv1.RoomEvent{
						{
							RoomId:   room.GetId(),
							SenderId: userID,
							Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
							Payload:  payload,
						},
					},
				}

				_, err := messageBusiness.SendEvents(ctx, msgReq, userID)
				s.NoError(err)
			}
		}

		// Verify each room has its messages
		for _, room := range rooms {
			historyReq := &chatv1.GetHistoryRequest{
				RoomId: room.GetId(),
				Limit:  10,
			}

			events, err := messageBusiness.GetHistory(ctx, historyReq, userID)
			s.NoError(err)
			s.GreaterOrEqual(len(events), 3)

			// Verify all messages belong to this room
			for _, event := range events {
				s.Equal(room.GetId(), event.GetRoomId())
			}
		}
	})
}

func (s *IntegrationTestSuite) TestRoleBasedPermissions() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _, subscriptionSvc := s.setupBusinessLayer(ctx, svc)

		ownerID := util.IDString()
		adminID := util.IDString()
		memberID := util.IDString()

		// Create room
		createReq := &chatv1.CreateRoomRequest{
			Name:      "Permission Test Room",
			IsPrivate: false,
			Members:   []string{adminID, memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, createReq, ownerID)
		s.NoError(err)

		// Promote one member to admin
		updateRoleReq := &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:    room.GetId(),
			ProfileId: adminID,
			Roles:     []string{"admin"},
		}

		err = roomBusiness.UpdateSubscriptionRole(ctx, updateRoleReq, ownerID)
		s.NoError(err)

		// Verify roles
		hasRole, err := subscriptionSvc.HasRole(ctx, ownerID, room.GetId(), repository.RoleOwner)
		s.NoError(err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(ctx, adminID, room.GetId(), repository.RoleAdmin)
		s.NoError(err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(ctx, memberID, room.GetId(), repository.RoleMember)
		s.NoError(err)
		s.True(hasRole)

		// Admin should be able to update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated by Admin",
		}

		_, err = roomBusiness.UpdateRoom(ctx, updateReq, adminID)
		s.NoError(err)

		// Member should not be able to update room
		updateReq.Name = "Updated by Member"
		_, err = roomBusiness.UpdateRoom(ctx, updateReq, memberID)
		s.Error(err)

		// Only owner should be able to delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, adminID)
		s.Error(err) // Admin cannot delete

		err = roomBusiness.DeleteRoom(ctx, deleteReq, ownerID)
		s.NoError(err) // Owner can delete
	})
}

func (s *IntegrationTestSuite) TestMessageDeletion() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, messageBusiness, _ := s.setupBusinessLayer(ctx, svc)

		userID := util.IDString()

		// Create room
		createReq := &chatv1.CreateRoomRequest{
			Name:      "Delete Test Room",
			IsPrivate: false,
		}

		room, err := roomBusiness.CreateRoom(ctx, createReq, userID)
		s.NoError(err)

		// Send messages
		messageIDs := []string{}
		for range 5 {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(10),
			})

			msgReq := &chatv1.SendEventRequest{
				Message: []*chatv1.RoomEvent{
					{
						RoomId:   room.GetId(),
						SenderId: userID,
						Type:     chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
						Payload:  payload,
					},
				},
			}

			acks, err := messageBusiness.SendEvents(ctx, msgReq, userID)
			s.NoError(err)
			messageIDs = append(messageIDs, acks[0].GetEventId())
		}

		dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)
		// Delete some messages via repository
		eventRepo := repository.NewRoomEventRepository(dbPool, svc.WorkManager())
		for i := range 3 {
			err := eventRepo.Delete(ctx, messageIDs[i])
			s.NoError(err)
		}

		// Verify deleted messages cannot be retrieved
		for i := range 3 {
			_, err := eventRepo.GetByID(ctx, messageIDs[i])
			s.Error(err)
		}

		// Verify remaining messages can be retrieved
		for i := 3; i < 5; i++ {
			msg, err := eventRepo.GetByID(ctx, messageIDs[i])
			s.NoError(err)
			s.NotNil(msg)
		}
	})
}

func (s *IntegrationTestSuite) TestSearchFunctionality() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _, _ := s.setupBusinessLayer(ctx, svc)

		userID := util.IDString()

		// Create rooms with specific names
		roomNames := []string{
			"Project Alpha Discussion",
			"Project Beta Planning",
			"General Chat",
			"Alpha Team Standup",
		}

		for _, name := range roomNames {
			createReq := &chatv1.CreateRoomRequest{
				Name:      name,
				IsPrivate: false,
			}

			_, err := roomBusiness.CreateRoom(ctx, createReq, userID)
			s.NoError(err)
		}

		// Search for "Alpha"
		searchReq := &chatv1.SearchRoomsRequest{
			Query: "Alpha",
		}

		results, err := roomBusiness.SearchRooms(ctx, searchReq, userID)
		s.NoError(err)
		s.GreaterOrEqual(len(results), 2)

		// Verify results contain "Alpha"
		alphaCount := 0
		for _, room := range results {
			if room.GetName() == "Project Alpha Discussion" || room.GetName() == "Alpha Team Standup" {
				alphaCount++
			}
		}
		s.GreaterOrEqual(alphaCount, 2)
	})
}
