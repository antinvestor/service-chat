package business_test

import (
	"context"
	"errors"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"buf.build/gen/go/antinvestor/profile/connectrpc/go/profile/v1/profilev1connect"
	profilev1 "buf.build/gen/go/antinvestor/profile/protocolbuffers/go/profile/v1"
	"connectrpc.com/connect"
	"github.com/antinvestor/service-chat/apps/default/service/business"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
)

type mockProfileClient struct {
	getByContactFunc func(context.Context, *connect.Request[profilev1.GetByContactRequest]) (*connect.Response[profilev1.GetByContactResponse], error)
}

func (m *mockProfileClient) GetByContact(
	ctx context.Context,
	req *connect.Request[profilev1.GetByContactRequest],
) (*connect.Response[profilev1.GetByContactResponse], error) {
	if m.getByContactFunc != nil {
		return m.getByContactFunc(ctx, req)
	}
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) AddAddress(
	context.Context,
	*connect.Request[profilev1.AddAddressRequest],
) (*connect.Response[profilev1.AddAddressResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) AddContact(
	context.Context,
	*connect.Request[profilev1.AddContactRequest],
) (*connect.Response[profilev1.AddContactResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) AddRelationship(
	context.Context,
	*connect.Request[profilev1.AddRelationshipRequest],
) (*connect.Response[profilev1.AddRelationshipResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) AddRoster(
	context.Context,
	*connect.Request[profilev1.AddRosterRequest],
) (*connect.Response[profilev1.AddRosterResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) CheckVerification(
	context.Context,
	*connect.Request[profilev1.CheckVerificationRequest],
) (*connect.Response[profilev1.CheckVerificationResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) Create(
	context.Context,
	*connect.Request[profilev1.CreateRequest],
) (*connect.Response[profilev1.CreateResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) CreateContact(
	context.Context,
	*connect.Request[profilev1.CreateContactRequest],
) (*connect.Response[profilev1.CreateContactResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) CreateContactVerification(
	context.Context,
	*connect.Request[profilev1.CreateContactVerificationRequest],
) (*connect.Response[profilev1.CreateContactVerificationResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) DeleteRelationship(
	context.Context,
	*connect.Request[profilev1.DeleteRelationshipRequest],
) (*connect.Response[profilev1.DeleteRelationshipResponse], error) {
	return nil, errors.New("not implemented")
}

//nolint:staticcheck,revive // Method name required by interface
func (m *mockProfileClient) GetById(
	context.Context,
	*connect.Request[profilev1.GetByIdRequest],
) (*connect.Response[profilev1.GetByIdResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) ListRelationship(
	context.Context,
	*connect.Request[profilev1.ListRelationshipRequest],
) (*connect.ServerStreamForClient[profilev1.ListRelationshipResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) Merge(
	context.Context,
	*connect.Request[profilev1.MergeRequest],
) (*connect.Response[profilev1.MergeResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) RemoveContact(
	context.Context,
	*connect.Request[profilev1.RemoveContactRequest],
) (*connect.Response[profilev1.RemoveContactResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) RemoveRoster(
	context.Context,
	*connect.Request[profilev1.RemoveRosterRequest],
) (*connect.Response[profilev1.RemoveRosterResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) Search(
	context.Context,
	*connect.Request[profilev1.SearchRequest],
) (*connect.ServerStreamForClient[profilev1.SearchResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) SearchRoster(
	context.Context,
	*connect.Request[profilev1.SearchRosterRequest],
) (*connect.ServerStreamForClient[profilev1.SearchRosterResponse], error) {
	return nil, errors.New("not implemented")
}

func (m *mockProfileClient) Update(
	context.Context,
	*connect.Request[profilev1.UpdateRequest],
) (*connect.Response[profilev1.UpdateResponse], error) {
	return nil, errors.New("not implemented")
}

func (s *RoomBusinessTestSuite) setupBusinessLayerWithProfileClient(
	ctx context.Context, svc *frame.Service, profileCli profilev1connect.ProfileServiceClient,
) business.RoomBusiness {
	workMan := svc.WorkManager()
	evtsMan := svc.EventsManager()
	dbPool := svc.DatastoreManager().GetPool(ctx, datastore.DefaultPoolName)

	roomRepo := repository.NewRoomRepository(ctx, dbPool, workMan)
	eventRepo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
	subRepo := repository.NewRoomSubscriptionRepository(ctx, dbPool, workMan)

	subscriptionSvc := business.NewSubscriptionService(svc, subRepo)
	messageBusiness := business.NewMessageBusiness(evtsMan, eventRepo, subRepo, subscriptionSvc)
	roomBusiness := business.NewRoomBusiness(
		svc,
		roomRepo,
		eventRepo,
		subRepo,
		subscriptionSvc,
		messageBusiness,
		profileCli,
		nil, // authz middleware
	)

	return roomBusiness
}

func (s *RoomBusinessTestSuite) TestCreateRoomWithValidationSuccess() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		validContactID := util.IDString()
		validProfileID := util.IDString()

		creatorID := util.IDString()
		creatorContactID := util.IDString()

		mockCli := &mockProfileClient{
			getByContactFunc: func(_ context.Context, req *connect.Request[profilev1.GetByContactRequest]) (*connect.Response[profilev1.GetByContactResponse], error) {
				if req.Msg.GetContact() == validContactID {
					return connect.NewResponse(&profilev1.GetByContactResponse{
						Data: &profilev1.ProfileObject{Id: validProfileID},
					}), nil
				}
				if req.Msg.GetContact() == creatorContactID {
					return connect.NewResponse(&profilev1.GetByContactResponse{
						Data: &profilev1.ProfileObject{Id: creatorID},
					}), nil
				}
				return nil, errors.New("not found")
			},
		}

		roomBusiness := s.setupBusinessLayerWithProfileClient(ctx, svc, mockCli)

		req := &chatv1.CreateRoomRequest{
			Name:      "Validation Room",
			IsPrivate: false,
			Members: []*commonv1.ContactLink{
				{ContactId: validContactID, ProfileId: validProfileID},
			},
		}

		room, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)
		s.NotNil(room)

		// Verify subscription created
		searchReq := &chatv1.SearchRoomSubscriptionsRequest{
			RoomId: room.GetId(),
		}
		subs, err := roomBusiness.SearchRoomSubscriptions(
			ctx,
			searchReq,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.NoError(t, err)

		found := false
		for _, sub := range subs {
			if sub.GetMember().GetProfileId() == validProfileID {
				found = true
				break
			}
		}
		s.True(found)
	})
}

func (s *RoomBusinessTestSuite) TestCreateRoomWithValidationFailure_ProfileMismatch() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		validContactID := util.IDString()
		actualProfileID := util.IDString()
		wrongProfileID := util.IDString()

		mockCli := &mockProfileClient{
			getByContactFunc: func(_ context.Context, _ *connect.Request[profilev1.GetByContactRequest]) (*connect.Response[profilev1.GetByContactResponse], error) {
				return connect.NewResponse(&profilev1.GetByContactResponse{
					Data: &profilev1.ProfileObject{Id: actualProfileID},
				}), nil
			},
		}

		roomBusiness := s.setupBusinessLayerWithProfileClient(ctx, svc, mockCli)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Mismatch Room",
			IsPrivate: false,
			Members: []*commonv1.ContactLink{
				{ContactId: validContactID, ProfileId: wrongProfileID},
			},
		}

		_, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.Error(t, err)
		s.Contains(err.Error(), "profile id mismatch")
	})
}

func (s *RoomBusinessTestSuite) TestCreateRoomWithValidationFailure_ContactNotFound() {
	s.WithTestDependencies(s.T(), func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)

		invalidContactID := util.IDString()

		mockCli := &mockProfileClient{
			getByContactFunc: func(_ context.Context, _ *connect.Request[profilev1.GetByContactRequest]) (*connect.Response[profilev1.GetByContactResponse], error) {
				return nil, errors.New("contact not found")
			},
		}

		roomBusiness := s.setupBusinessLayerWithProfileClient(ctx, svc, mockCli)

		creatorID := util.IDString()
		creatorContactID := util.IDString()
		req := &chatv1.CreateRoomRequest{
			Name:      "Invalid Contact Room",
			IsPrivate: false,
			Members: []*commonv1.ContactLink{
				{ContactId: invalidContactID},
			},
		}

		_, err := roomBusiness.CreateRoom(
			ctx,
			req,
			&commonv1.ContactLink{ProfileId: creatorID, ContactId: creatorContactID},
		)
		require.Error(t, err)
		s.Contains(err.Error(), "contact validation failed")
	})
}
