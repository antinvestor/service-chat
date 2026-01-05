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
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RoomBusinessTestSuite struct {
	tests.BaseTestSuite
}

func TestRoomBusinessTestSuite(t *testing.T) {
	suite.Run(t, new(RoomBusinessTestSuite))
}

func (s *RoomBusinessTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) business.RoomBusiness {
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness, nil)

	return roomBusiness
}

func (s *RoomBusinessTestSuite) TestCreateRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:        "Test Room",
			Description: "Test Description",
			IsPrivate:   false,
			Members: []*commonv1.ContactLink{
				&commonv1.ContactLink{ProfileId: util.IDString()},
				&commonv1.ContactLink{ProfileId: util.IDString()},
			},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.NotNil(room)
		s.Equal("Test Room", room.GetName())
		s.Equal("Test Description", room.GetDescription())
		s.False(room.GetIsPrivate())
	})
}

func (s *RoomBusinessTestSuite) TestCreateRoomWithoutName() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		req := &chatv1.CreateRoomRequest{
			Name: "",
		}

		_, err := roomBusiness.CreateRoom(ctx, req, &commonv1.ContactLink{ProfileId: util.IDString()})
		require.Error(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestGetRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Get the room
		retrieved, err := roomBusiness.GetRoom(
			ctx,
			created.GetId(),
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Equal(created.GetId(), retrieved.GetId())
		s.Equal(created.GetName(), retrieved.GetName())
	})
}

func (s *RoomBusinessTestSuite) TestGetRoomAccessDenied() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		otherUserID := util.IDString()
		otherUserContactID := util.IDString()

		req := &chatv1.CreateRoomRequest{
			Name:      "Private Room",
			IsPrivate: true,
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Try to get room as non-member
		_, err = roomBusiness.GetRoom(
			ctx,
			created.GetId(),
			&commonv1.ContactLink{ProfileId: otherUserID, ContactId: otherUserContactID},
		)
		require.Error(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestUpdateRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Original Name",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.GetId(),
			Name:   "Updated Name",
			Topic:  "Updated Description",
		}

		updated, err := roomBusiness.UpdateRoom(
			ctx,
			updateReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.Equal("Updated Name", updated.GetName())
		s.Equal("Updated Description", updated.GetDescription())
	})
}

func (s *RoomBusinessTestSuite) TestUpdateRoomUnauthorized() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Try to update as non-admin member
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.GetId(),
			Name:   "Hacked Name",
		}

		_, err = roomBusiness.UpdateRoom(
			ctx,
			updateReq,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
		)
		require.Error(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestDeleteRoom() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Room to Delete",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: created.GetId(),
		}

		err = roomBusiness.DeleteRoom(
			ctx,
			deleteReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Verify deletion
		_, err = roomBusiness.GetRoom(
			ctx,
			created.GetId(),
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.Error(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestAddRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		newMemberID := util.IDString()
		newMemberContactID := util.IDString()

		// Create room
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Add new member
		addReq := &chatv1.AddRoomSubscriptionsRequest{
			RoomId: created.GetId(),
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

		// Verify member added
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: created.GetId(),
		}
		subs, err := roomBusiness.SearchRoomSubscriptions(
			ctx,
			searchReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.GreaterOrEqual(len(subs), 2) // Creator + new member
	})
}

func (s *RoomBusinessTestSuite) TestRemoveRoomSubscriptions() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		// Create room with member
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Get subscription ID
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: created.GetId(),
		}
		searchResp, err := roomBusiness.SearchRoomSubscriptions(
			ctx,
			searchReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		var subscriptionID string
		for _, sub := range searchResp {
			if sub.GetMember().GetProfileId() == memberID {
				subscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, subscriptionID)

		// Remove member
		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:         created.GetId(),
			SubscriptionId: []string{subscriptionID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(
			ctx,
			removeReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Verify member removed (should not have access)
		_, err = roomBusiness.GetRoom(
			ctx,
			created.GetId(),
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
		)
		require.Error(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestUpdateSubscriptionRole() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		// Create room with member
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		created, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Get subscription ID
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: created.GetId(),
		}
		searchResp, err := roomBusiness.SearchRoomSubscriptions(
			ctx,
			searchReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		var subscriptionID string
		for _, sub := range searchResp {
			if sub.GetMember().GetProfileId() == memberID {
				subscriptionID = sub.GetId()
				break
			}
		}
		require.NotEmpty(t, subscriptionID)

		// Promote member to admin
		updateReq := &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:         created.GetId(),
			SubscriptionId: subscriptionID,
			Roles:          []string{"admin"},
		}

		err = roomBusiness.UpdateSubscriptionRole(
			ctx,
			updateReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Verify role updated - member should now be able to update room
		roomUpdateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.GetId(),
			Name:   "Updated by Admin",
		}

		_, err = roomBusiness.UpdateRoom(
			ctx,
			roomUpdateReq,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
		)
		require.NoError(t, err)
	})
}

func (s *RoomBusinessTestSuite) TestSearchRooms() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		roomBusiness := s.setupBusinessLayer(ctx, svc)

		userID := util.IDString()
		userContactID := util.IDString()

		// Create multiple rooms
		rooms := []string{"Alpha Room", "Beta Room", "Gamma Room"}
		for _, name := range rooms {
			req := &chatv1.CreateRoomRequest{
				Name:      name,
				IsPrivate: false,
			}
			_, err := roomBusiness.CreateRoom(
				ctx,
				req,
				&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
			)
			require.NoError(t, err)
		}

		// Search for rooms
		searchReq := &chatv1.SearchRoomsRequest{
			Query: "Alpha",
		}

		results, err := roomBusiness.SearchRooms(
			ctx,
			searchReq,
			&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
		)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(results), 1)

		found := false
		for _, room := range results {
			if room.GetName() == "Alpha Room" {
				found = true
				break
			}
		}
		require.True(t, found)
	})
}
