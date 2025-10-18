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
)

type RoomBusinessTestSuite struct {
	BaseTestSuite
}

func TestRoomBusinessTestSuite(t *testing.T) {
	suite.Run(t, new(RoomBusinessTestSuite))
}

func (s *RoomBusinessTestSuite) setupBusinessLayer(svc *frame.Service) (business.RoomBusiness, business.MessageBusiness) {
	roomRepo := repository.NewRoomRepository(svc)
	eventRepo := repository.NewRoomEventRepository(svc)
	subRepo := repository.NewRoomSubscriptionRepository(svc)
	outboxRepo := repository.NewRoomOutboxRepository(svc)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return roomBusiness, messageBusiness
}

func (s *RoomBusinessTestSuite) TestCreateRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:        "Test Room",
			Description: "Test Description",
			IsPrivate:   false,
			Members:     []string{util.IDString(), util.IDString()},
		}

		room, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)
		s.NotNil(room)
		s.Equal("Test Room", room.Name)
		s.Equal("Test Description", room.Description)
		s.False(room.IsPrivate)
	})
}

func (s *RoomBusinessTestSuite) TestCreateRoomWithoutName() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		req := &chatv1.CreateRoomRequest{
			Name: "",
		}

		_, err := roomBusiness.CreateRoom(ctx, req, util.IDString())
		s.Error(err)
	})
}

func (s *RoomBusinessTestSuite) TestGetRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Get the room
		retrieved, err := roomBusiness.GetRoom(ctx, created.Id, creatorID)
		s.NoError(err)
		s.Equal(created.Id, retrieved.Id)
		s.Equal(created.Name, retrieved.Name)
	})
}

func (s *RoomBusinessTestSuite) TestGetRoomAccessDenied() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		otherUserID := util.IDString()

		req := &chatv1.CreateRoomRequest{
			Name:      "Private Room",
			IsPrivate: true,
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Try to get room as non-member
		_, err = roomBusiness.GetRoom(ctx, created.Id, otherUserID)
		s.Error(err)
	})
}

func (s *RoomBusinessTestSuite) TestUpdateRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Original Name",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Update room
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.Id,
			Name:   "Updated Name",
			Topic:  "Updated Description",
		}

		updated, err := roomBusiness.UpdateRoom(ctx, updateReq, creatorID)
		s.NoError(err)
		s.Equal("Updated Name", updated.Name)
		s.Equal("Updated Description", updated.Description)
	})
}

func (s *RoomBusinessTestSuite) TestUpdateRoomUnauthorized() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		memberID := util.IDString()

		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Try to update as non-admin member
		updateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.Id,
			Name:   "Hacked Name",
		}

		_, err = roomBusiness.UpdateRoom(ctx, updateReq, memberID)
		s.Error(err)
	})
}

func (s *RoomBusinessTestSuite) TestDeleteRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Room to Delete",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Delete room
		deleteReq := &chatv1.DeleteRoomRequest{
			RoomId: created.Id,
		}

		err = roomBusiness.DeleteRoom(ctx, deleteReq, creatorID)
		s.NoError(err)

		// Verify deletion
		_, err = roomBusiness.GetRoom(ctx, created.Id, creatorID)
		s.Error(err)
	})
}

func (s *RoomBusinessTestSuite) TestAddRoomSubscriptions() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		newMemberID := util.IDString()

		// Create room
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Add new member
		addReq := &chatv1.AddRoomSubscriptionsRequest{
			RoomId: created.Id,
			Members: []*chatv1.RoomSubscription{
				{
					ProfileId: newMemberID,
					Roles:     []string{"member"},
				},
			},
		}

		err = roomBusiness.AddRoomSubscriptions(ctx, addReq, creatorID)
		s.NoError(err)

		// Verify member added
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: created.Id,
		}
		subs, err := roomBusiness.SearchRoomSubscriptions(ctx, searchReq, creatorID)
		s.NoError(err)
		s.GreaterOrEqual(len(subs), 2) // Creator + new member
	})
}

func (s *RoomBusinessTestSuite) TestRemoveRoomSubscriptions() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		memberID := util.IDString()

		// Create room with member
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Remove member
		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:     created.Id,
			ProfileIds: []string{memberID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(ctx, removeReq, creatorID)
		s.NoError(err)

		// Verify member removed (should not have access)
		_, err = roomBusiness.GetRoom(ctx, created.Id, memberID)
		s.Error(err)
	})
}

func (s *RoomBusinessTestSuite) TestUpdateSubscriptionRole() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		creatorID := util.IDString()
		memberID := util.IDString()

		// Create room with member
		req := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		created, err := roomBusiness.CreateRoom(ctx, req, creatorID)
		s.NoError(err)

		// Promote member to admin
		updateReq := &chatv1.UpdateSubscriptionRoleRequest{
			RoomId:    created.Id,
			ProfileId: memberID,
			Roles:     []string{"admin"},
		}

		err = roomBusiness.UpdateSubscriptionRole(ctx, updateReq, creatorID)
		s.NoError(err)

		// Verify role updated - member should now be able to update room
		roomUpdateReq := &chatv1.UpdateRoomRequest{
			RoomId: created.Id,
			Name:   "Updated by Admin",
		}

		_, err = roomBusiness.UpdateRoom(ctx, roomUpdateReq, memberID)
		s.NoError(err)
	})
}

func (s *RoomBusinessTestSuite) TestSearchRooms() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		roomBusiness, _ := s.setupBusinessLayer(svc)

		userID := util.IDString()

		// Create multiple rooms
		rooms := []string{"Alpha Room", "Beta Room", "Gamma Room"}
		for _, name := range rooms {
			req := &chatv1.CreateRoomRequest{
				Name:      name,
				IsPrivate: false,
			}
			_, err := roomBusiness.CreateRoom(ctx, req, userID)
			s.NoError(err)
		}

		// Search for rooms
		searchReq := &chatv1.SearchRoomsRequest{
			Query: "Alpha",
		}

		results, err := roomBusiness.SearchRooms(ctx, searchReq, userID)
		s.NoError(err)
		s.GreaterOrEqual(len(results), 1)

		found := false
		for _, room := range results {
			if room.Name == "Alpha Room" {
				found = true
				break
			}
		}
		s.True(found)
	})
}
