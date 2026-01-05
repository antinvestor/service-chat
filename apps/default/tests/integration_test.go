package tests_test

import (
	"context"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IntegrationTestSuite struct {
	tests.BaseTestSuite
}

func TestIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) (business.RoomBusiness, business.MessageBusiness, business.SubscriptionService) {
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness, nil)

	return roomBusiness, messageBusiness, subscriptionSvc
}

func (s *IntegrationTestSuite) TestCompleteRoomLifecycle() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness, subscriptionSvc := s.setupBusinessLayer(ctx, svc)

		// 1. Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		member1ID := util.IDString()
		member1ContactID := util.IDString()
		member2ID := util.IDString()
		member2ContactID := util.IDString()

		createReq := &chatv1.CreateRoomRequest{
			Name:        "Integration Test Room",
			Description: "Testing complete workflow",
			IsPrivate:   false,
			Members: []*commonv1.ContactLink{
				&commonv1.ContactLink{ProfileId: member1ID, ContactId: member1ContactID},
				&commonv1.ContactLink{ProfileId: member2ID, ContactId: member2ContactID},
			},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			createReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.NotNil(room)

		// 2. Verify all members have access
		accessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		accessMap, err = subscriptionSvc.HasAccess(ctx, &commonv1.ContactLink{ProfileId: member1ID, ContactId: member1ContactID}, room.GetId())
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// 3. Send messages
		for range 5 {
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

			acks, sendErr := messageBusiness.SendEvents(
				ctx,
				msgReq,
				&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			)
			require.NoError(t, sendErr)
			s.Len(acks, 1)
		}

		// 4. Get history
		historyReq := &chatv1.GetHistoryRequest{
			RoomId: room.GetId(),
			Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
		}

		events, err := messageBusiness.GetHistory(ctx, historyReq, &commonv1.ContactLink{ProfileId: member1ID, ContactId: member1ContactID})
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), 5)

		// 5. Update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated Room Name",
			Topic:  "Updated Description",
		}

		updated, err := roomBusiness.UpdateRoom(
			ctx,
			updateReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Equal("Updated Room Name", updated.GetName())

		// 6. Add new member
		newMemberID := util.IDString()
		newMemberContactID := util.IDString()
		addReq := &chatv1.AddRoomSubscriptionsRequest{
			RoomId: room.GetId(),
			Members: []*chatv1.RoomSubscription{
				{
					Member: &commonv1.ContactLink{ProfileId: newMemberID, ContactId: newMemberContactID},
					Roles:  []string{"member"},
				},
			},
		}

		err = roomBusiness.AddRoomSubscriptions(
			ctx,
			addReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// 7. Verify new member has access
		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: newMemberID, ContactId: newMemberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// 8. Remove member - first find the subscription ID
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
		}

		searchResp, err := roomBusiness.SearchRoomSubscriptions(ctx, searchReq, &commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID})
		require.NoError(t, err)

		var member2SubscriptionID string
		for _, sub := range searchResp {
			if sub.GetMember().GetProfileId() == member2ID {
				member2SubscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, member2SubscriptionID)

		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:         room.GetId(),
			SubscriptionId: []string{member2SubscriptionID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(
			ctx,
			removeReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// 9. Verify removed member no longer has access
		accessMap, err = subscriptionSvc.HasAccess(ctx, &commonv1.ContactLink{ProfileId: member2ID, ContactId: member2ContactID}, room.GetId())
		require.NoError(t, err)

		// Check that no subscriptions are active (all values should be false)
		for _, hasAccess := range accessMap {
			s.False(hasAccess, "Removed member should not have active access")
		}

		// 10. Delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(
			ctx,
			deleteReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// 11. Verify room is deleted
		_, err = roomBusiness.GetRoom(
			ctx,
			room.GetId(),
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.Error(t, err)
	})
}

func (s *IntegrationTestSuite) TestMultiRoomMessaging() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness, messageBusiness, _ := s.setupBusinessLayer(ctx, svc)

		userID := util.IDString()
		userContactID := util.IDString()

		// Create multiple rooms
		rooms := []*chatv1.Room{}
		for range 3 {
			createReq := &chatv1.CreateRoomRequest{
				Name:      util.RandomString(10),
				IsPrivate: false,
			}

			room, err := roomBusiness.CreateRoom(
				ctx,
				createReq,
				&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
			)
			require.NoError(t, err)
			rooms = append(rooms, room)
		}

		// Send messages to each room
		for _, room := range rooms {
			for range 3 {
				msgReq := &chatv1.SendEventRequest{
					Event: []*chatv1.RoomEvent{
						{
							RoomId: room.GetId(),
							Source: &commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
							Type:   chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
							Payload: &chatv1.Payload{
								Data: &chatv1.Payload_Text{Text: &chatv1.TextContent{Body: "test message"}},
							},
						},
					},
				}

				_, err := messageBusiness.SendEvents(
					ctx,
					msgReq,
					&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
				)
				require.NoError(t, err)
			}
		}

		// Verify each room has its messages
		for _, room := range rooms {
			historyReq := &chatv1.GetHistoryRequest{
				RoomId: room.GetId(),
				Cursor: &commonv1.PageCursor{Limit: 10, Page: ""},
			}

			events, err := messageBusiness.GetHistory(
				ctx,
				historyReq,
				&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
			)
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
		ownerContactID := util.IDString()
		adminID := util.IDString()
		adminContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		// Create room
		createReq := &chatv1.CreateRoomRequest{
			Name:      "Permission Test Room",
			IsPrivate: false,
			Members: []*commonv1.ContactLink{
				&commonv1.ContactLink{ProfileId: adminID, ContactId: adminContactID},
				&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			},
		}

		room, err := roomBusiness.CreateRoom(ctx, createReq, &commonv1.ContactLink{ProfileId: ownerID, ContactId: ownerContactID})
		require.NoError(t, err)

		// Promote one member to admin - first find the subscription ID
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
		}

		searchResp, err := roomBusiness.SearchRoomSubscriptions(ctx, searchReq, &commonv1.ContactLink{ProfileId: ownerID, ContactId: ownerContactID})
		require.NoError(t, err)

		var adminSubscriptionID string
		for _, sub := range searchResp {
			if sub.GetMember().GetProfileId() == adminID {
				adminSubscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, adminSubscriptionID)

		updateRoleReq := &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:         room.GetId(),
			SubscriptionId: adminSubscriptionID,
			Roles:          []string{"admin"},
		}

		err = roomBusiness.UpdateSubscriptionRole(ctx, updateRoleReq, &commonv1.ContactLink{ProfileId: ownerID, ContactId: ownerContactID})
		require.NoError(t, err)

		// Verify roles
		hasRole, err := subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: ownerID, ContactId: ownerContactID},
			room.GetId(),
			repository.RoleOwner,
		)
		require.NoError(t, err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: adminID, ContactId: adminContactID},
			room.GetId(),
			repository.RoleAdmin,
		)
		require.NoError(t, err)
		s.True(hasRole)

		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
			repository.RoleMember,
		)
		require.NoError(t, err)
		s.True(hasRole)

		// Admin should be able to update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: room.GetId(),
			Name:   "Updated by Admin",
		}

		_, err = roomBusiness.UpdateRoom(ctx, updateReq, &commonv1.ContactLink{ProfileId: adminID, ContactId: adminContactID})
		require.NoError(t, err)

		// Member should not be able to update room
		updateReq.Name = "Updated by Member"
		_, err = roomBusiness.UpdateRoom(
			ctx,
			updateReq,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
		)
		require.Error(t, err)

		// Only owner should be able to delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: room.GetId(),
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, &commonv1.ContactLink{ProfileId: adminID, ContactId: adminContactID})
		require.Error(t, err) // Admin cannot delete

		err = roomBusiness.DeleteRoom(ctx, deleteReq, &commonv1.ContactLink{ProfileId: ownerID, ContactId: ownerContactID})
		require.NoError(t, err) // Owner can delete
	})
}
