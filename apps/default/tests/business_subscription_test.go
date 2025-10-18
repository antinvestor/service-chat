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

type SubscriptionServiceTestSuite struct {
	BaseTestSuite
}

func TestSubscriptionServiceTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionServiceTestSuite))
}

func (s *SubscriptionServiceTestSuite) setupBusinessLayer(svc *frame.Service) (business.SubscriptionService, business.RoomBusiness) {
	roomRepo := repository.NewRoomRepository(svc)
	eventRepo := repository.NewRoomEventRepository(svc)
	subRepo := repository.NewRoomSubscriptionRepository(svc)
	outboxRepo := repository.NewRoomOutboxRepository(svc)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(svc, eventRepo, outboxRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness)

	return subscriptionSvc, roomBusiness
}

func (s *SubscriptionServiceTestSuite) TestHasAccess() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		memberID := util.IDString()
		nonMemberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Creator should have access
		hasAccess, err := subscriptionSvc.HasAccess(ctx, creatorID, room.Id)
		s.NoError(err)
		s.True(hasAccess)

		// Member should have access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, memberID, room.Id)
		s.NoError(err)
		s.True(hasAccess)

		// Non-member should not have access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, nonMemberID, room.Id)
		s.NoError(err)
		s.False(hasAccess)
	})
}

func (s *SubscriptionServiceTestSuite) TestHasRole() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Creator should have owner role
		hasRole, err := subscriptionSvc.HasRole(ctx, creatorID, room.Id, repository.RoleOwner)
		s.NoError(err)
		s.True(hasRole)

		// Member should not have owner role
		hasRole, err = subscriptionSvc.HasRole(ctx, memberID, room.Id, repository.RoleOwner)
		s.NoError(err)
		s.False(hasRole)

		// Member should have member role
		hasRole, err = subscriptionSvc.HasRole(ctx, memberID, room.Id, repository.RoleMember)
		s.NoError(err)
		s.True(hasRole)
	})
}

func (s *SubscriptionServiceTestSuite) TestGetSubscribedRoomIDs() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(svc)

		// Create multiple rooms
		userID := util.IDString()
		roomCount := 5

		for i := 0; i < roomCount; i++ {
			roomReq := &chatv1.CreateRoomRequest{
				Name:      util.RandomString(10),
				IsPrivate: false,
			}

			_, err := roomBusiness.CreateRoom(ctx, roomReq, userID)
			s.NoError(err)
		}

		// Get subscribed room IDs
		roomIDs, err := subscriptionSvc.GetSubscribedRoomIDs(ctx, userID)
		s.NoError(err)
		s.GreaterOrEqual(len(roomIDs), roomCount)
	})
}

func (s *SubscriptionServiceTestSuite) TestIsRoomMemberViaHasAccess() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(svc)

		// Create room
		creatorID := util.IDString()
		memberID := util.IDString()
		nonMemberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Check membership via HasAccess
		hasAccess, err := subscriptionSvc.HasAccess(ctx, creatorID, room.Id)
		s.NoError(err)
		s.True(hasAccess)

		hasAccess, err = subscriptionSvc.HasAccess(ctx, memberID, room.Id)
		s.NoError(err)
		s.True(hasAccess)

		hasAccess, err = subscriptionSvc.HasAccess(ctx, nonMemberID, room.Id)
		s.NoError(err)
		s.False(hasAccess)
	})
}

func (s *SubscriptionServiceTestSuite) TestAccessAfterRemoval() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(svc)

		// Create room with member
		creatorID := util.IDString()
		memberID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []string{memberID},
		}

		room, err := roomBusiness.CreateRoom(ctx, roomReq, creatorID)
		s.NoError(err)

		// Verify member has access
		hasAccess, err := subscriptionSvc.HasAccess(ctx, memberID, room.Id)
		s.NoError(err)
		s.True(hasAccess)

		// Remove member
		removeReq := &chatv1.RemoveRoomSubscriptionsRequest{
			RoomId:     room.Id,
			ProfileIds: []string{memberID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(ctx, removeReq, creatorID)
		s.NoError(err)

		// Verify member no longer has access
		hasAccess, err = subscriptionSvc.HasAccess(ctx, memberID, room.Id)
		s.NoError(err)
		s.False(hasAccess)
	})
}
