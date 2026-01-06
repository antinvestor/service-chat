package business_test

import (
	"context"
	"errors"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"connectrpc.com/connect"
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
		_, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: nonMemberID, ContactId: nonMemberContactID},
			room.GetId(),
		)
		require.Error(t, err)
		s.Equal(connect.CodePermissionDenied, func() *connect.Error {
			target := &connect.Error{}
			_ = errors.As(err, &target)
			return target
		}().Code())
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
			3, // roleOwnerLevel
		)
		require.NoError(t, err)
		s.NotNil(hasRole)

		// Member should not have owner role
		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
			3, // roleOwnerLevel
		)
		s.Error(err)
		s.Nil(hasRole)

		// Member should have member role
		hasRole, err = subscriptionSvc.HasRole(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
			1, // roleMemberLevel
		)
		require.NoError(t, err)
		s.NotNil(hasRole)
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
		creatorAccessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(creatorAccessMap)

		memberAccessMap, err := subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: memberID, ContactId: memberContactID},
			room.GetId(),
		)
		require.NoError(t, err)
		s.NotEmpty(memberAccessMap)

		_, err = subscriptionSvc.HasAccess(
			ctx,
			&commonv1.ContactLink{ProfileId: nonMemberID, ContactId: nonMemberContactID},
			room.GetId(),
		)
		require.Error(t, err)
		s.Equal(connect.CodePermissionDenied, func() *connect.Error {
			target := &connect.Error{}
			_ = errors.As(err, &target)
			return target
		}().Code())
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

		// Check that no subscriptions are active (filter out inactive ones)
		activeSubscriptions := 0
		for _, sub := range accessMap {
			if sub.IsActive() {
				activeSubscriptions++
			}
		}
		s.Equal(0, activeSubscriptions, "Removed member should not have active access")
	})
}
