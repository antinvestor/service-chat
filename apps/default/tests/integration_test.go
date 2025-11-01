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
	"github.com/stretchr/testify/require"
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
	evtsMan := svc.EventsManager(ctx)
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)
	outboxRepo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return roomBusiness, messageBusiness, subscriptionSvc
}

func (s *IntegrationTestSuite) TestCompleteRoomLifecycle() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
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
		require.NoError(t, err)
		s.NotNil(room)

		// 2. Verify all members have access
		hasAccess, err := subscriptionSvc.HasAccess(ctx, creatorID, room.GetId())
		require.NoError(t, err)
		s.True(hasAccess)

		hasAccess, err = subscriptionSvc.HasAccess(ctx, member1ID, room.GetId())
		require.NoError(t, err)
		s.True(hasAccess)

		// 3. Send messages
		for range 5 {
			payload, _ := structpb.NewStruct(map[string]interface{}{
				"text": util.RandomString(20),
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
			s.Len(acks, 1)
		}

		// 4. Get history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Limit:  10,
		}

		events, err := messageBusiness.GetHistory(ctx, historyReq, member1ID)
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), 5)

		// 5. Update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated Room Name",
			Topic:  "Updated Description",
		}

		updated, err := roomBusiness.UpdateRoom(ctx, updateReq, creatorID)
		require.NoError(t, err)
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
		require.NoError(t, err)

		// 7. Verify new member has access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, newMemberID, room.GetId())
		require.NoError(t, err)
		s.True(hasAccess)

		// 8. Remove member
		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:     room.GetId(),
			ProfileIds: []string{member2ID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(ctx, removeReq, creatorID)
		require.NoError(t, err)

		// 9. Verify removed member no longer has access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, member2ID, room.GetId())
		require.NoError(t, err)
		s.False(hasAccess)

		// 10. Delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, creatorID)
		require.NoError(t, err)

		// 11. Verify room is deleted
		_, err = roomBusiness.GetRoom(ctx, room.GetId(), creatorID)
		require.Error(t, err)
	})
}

func (s *IntegrationTestSuite) TestMultiRoomMessaging() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
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
			require.NoError(t, err)
			rooms = append(rooms, room)
		}

		// Send messages to each room
		for _, room := range rooms {
			for range 3 {
				payload, _ := structpb.NewStruct(map[string]interface{}{
					"text": util.RandomString(15),
				})

				msgReq := &chatv1.SendEventRequest{
					Event: []*chatv1.RoomEvent{
						{
							RoomId:   room.GetId(),
							SenderId: userID,
							Type:     chatv1.RoomEventType_TEXT,
							Payload:  payload,
						},
					},
				}

				_, err := messageBusiness.SendEvents(ctx, msgReq, userID)
				require.NoError(t, err)
			}
		}

		// Verify each room has its messages
		for _, room := range rooms {
			historyReq := &chatv1.GetHistoryRequest{
				RoomId: room.GetId(),
				Limit:  10,
			}

			events, err := messageBusiness.GetHistory(ctx, historyReq, userID)
			require.NoError(t, err)
			s.GreaterOrEqual(len(events), 3)

			// Verify all messages belong to this room
			for _, event := range events {
				s.Equal(room.GetId(), event.GetRoomId())
			}
		}
	})
}

func (s *IntegrationTestSuite) TestRoleBasedPermissions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
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
		require.NoError(t, err)

		// Promote one member to admin
		updateRoleReq := &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:    room.GetId(),
			ProfileId: adminID,
			Roles:     []string{"admin"},
		}

		err = roomBusiness.UpdateSubscriptionRole(ctx, updateRoleReq, ownerID)
		require.NoError(t, err)

		// Verify roles
		hasRole, err := subscriptionSvc.HasRole(ctx, ownerID, room.GetId(), repository.RoleOwner)
		require.NoError(t, err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(ctx, adminID, room.GetId(), repository.RoleAdmin)
		require.NoError(t, err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(ctx, memberID, room.GetId(), repository.RoleMember)
		require.NoError(t, err)
		s.True(hasRole)

		// Admin should be able to update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated by Admin",
		}

		_, err = roomBusiness.UpdateRoom(ctx, updateReq, adminID)
		require.NoError(t, err)

		// Member should not be able to update room
		updateReq.Name = "Updated by Member"
		_, err = roomBusiness.UpdateRoom(ctx, updateReq, memberID)
		require.Error(t, err)

		// Only owner should be able to delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, adminID)
		require.Error(t, err) // Admin cannot delete

		err = roomBusiness.DeleteRoom(ctx, deleteReq, ownerID)
		require.NoError(t, err) // Owner can delete
	})
}
