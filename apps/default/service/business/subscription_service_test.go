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

type SubscriptionServiceTestSuite struct {
	tests.BaseTestSuite
}

func TestSubscriptionServiceTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionServiceTestSuite))
}

func (s *SubscriptionServiceTestSuite) setupBusinessLayer(
	ctx context.Context, svc *frame.Service,
) (business.SubscriptionService, business.RoomBusiness) {
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(svc, roomRepo, eventRepo, subRepo, subscriptionSvc, messageBusiness, nil)

	return subscriptionSvc, roomBusiness
}

func (s *SubscriptionServiceTestSuite) TestHasAccess() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()
		nonMemberID := util.IDString()
		nonMemberContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Creator should have access
		accessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// Member should have access
		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// Non-member should not have access
		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: nonMemberID, ContactId: nonMemberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.Empty(accessMap)
	})
}

func (s *SubscriptionServiceTestSuite) TestHasRole() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Creator should have owner role
		hasRole, err := subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			room.GetId(),
			repository.RoleOwner,
		)
		require.NoError(t, err)
		s.True(hasRole)

		// Member should not have owner role
		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
			repository.RoleOwner,
		)
		require.NoError(t, err)
		s.False(hasRole)

		// Member should have member role
		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
			repository.RoleMember,
		)
		require.NoError(t, err)
		s.True(hasRole)
	})
}

func (s *SubscriptionServiceTestSuite) TestGetSubscribedRoomIDs() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create multiple rooms
		userID := util.IDString()
		userContactID := util.IDString()
		roomCount := 5

		for range roomCount {
			roomReq := &chatv1.CreateRoomRequest{
				Name:      util.RandomString(10),
				IsPrivate: false,
			}

			_, err := roomBusiness.CreateRoom(
				ctx,
				roomReq,
				&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
			)
			require.NoError(t, err)
		}

		// Get subscribed room IDs
		roomIDs, err := subscriptionSvc.GetSubscribedRoomIDs(
			ctx,
			&commonv1.ContactLink{ProfileId: userID, ContactId: userContactID},
		)
		require.NoError(t, err)
		s.GreaterOrEqual(len(roomIDs), roomCount)
	})
}

func (s *SubscriptionServiceTestSuite) TestIsRoomMemberViaHasAccess() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()
		nonMemberID := util.IDString()
		nonMemberContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Check membership via HasAccess
		accessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: nonMemberID, ContactId: nonMemberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.Empty(accessMap)
	})
}

func (s *SubscriptionServiceTestSuite) TestAccessAfterRemoval() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		subscriptionSvc, roomBusiness := s.setupBusinessLayer(ctx, svc)

		// Create room with member
		creatorID := util.IDString()
		creatorContactID := util.IDString()
		memberID := util.IDString()
		memberContactID := util.IDString()

		roomReq := &chatv1.CreateRoomRequest{
			Name:      "Test Room",
			IsPrivate: false,
			Members:   []*commonv1.ContactLink{&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID}},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			roomReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Verify member has access
		accessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(accessMap)

		// Get subscription ID
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
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
			RoomId:         room.GetId(),
			SubscriptionId: []string{subscriptionID},
		}

		err = roomBusiness.RemoveRoomSubscriptions(
			ctx,
			removeReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		// Verify member no longer has access
		accessMap, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.Empty(accessMap)
	})
}
