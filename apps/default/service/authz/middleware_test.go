package authz_test

import (
	"context"
	"errors"
	"testing"

	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/authz"
	"github.com/antinvestor/service-chat/apps/default/service/authz/mock"
	"github.com/pitabwire/frame/security"
	"github.com/pitabwire/frame/security/authorizer"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MiddlewareTestSuite struct {
	suite.Suite
	mockService *mock.MockAuthzService
	middleware  authz.Middleware
}

func TestMiddlewareTestSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareTestSuite))
}

func (s *MiddlewareTestSuite) SetupTest() {
	s.mockService = mock.NewMockAuthzService()
	s.middleware = authz.NewMiddleware(s.mockService)
}

func (s *MiddlewareTestSuite) TearDownTest() {
	s.mockService.Reset()
}

func (s *MiddlewareTestSuite) actor(profileID string) *commonv1.ContactLink {
	return &commonv1.ContactLink{
		ProfileId: profileID,
		ContactId: util.IDString(),
	}
}

// CanViewRoom tests.
func (s *MiddlewareTestSuite) TestCanViewRoom() {
	testCases := []struct {
		name            string
		role            string
		addMembership   bool
		shouldBeAllowed bool
	}{
		{"MemberCanView", authz.RoleMember, true, true},
		{"AdminCanView", authz.RoleAdmin, true, true},
		{"OwnerCanView", authz.RoleOwner, true, true},
		{"GuestCanView", authz.RoleGuest, true, true},
		{"NonMemberDenied", "", false, false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			ctx := context.Background()
			roomID := util.IDString()
			profileID := util.IDString()

			if tc.addMembership {
				err := s.mockService.AddRoomMember(roomID, profileID, tc.role)
				s.Require().NoError(err)
			}

			err := s.middleware.CanViewRoom(ctx, s.actor(profileID), roomID)

			if tc.shouldBeAllowed {
				s.Require().NoError(err)
			} else {
				s.Require().Error(err)
				s.Require().True(errors.Is(err, authorizer.ErrPermissionDenied))
			}
		})
	}
}

func (s *MiddlewareTestSuite) TestCanViewRoom_EmptyProfileIDDenied() {
	ctx := context.Background()
	roomID := util.IDString()

	err := s.middleware.CanViewRoom(ctx, s.actor(""), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrInvalidSubject))
}

// CanSendMessage tests.
func (s *MiddlewareTestSuite) TestCanSendMessage_MemberCanSend() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	err = s.middleware.CanSendMessage(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanSendMessage_AdminCanSend() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanSendMessage(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanSendMessage_OwnerCanSend() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanSendMessage(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanSendMessage_GuestCannotSend() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	// Guest (viewer) should not be able to send messages
	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleGuest)
	require.NoError(s.T(), err)

	err = s.middleware.CanSendMessage(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

func (s *MiddlewareTestSuite) TestCanSendMessage_NonMemberDenied() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.middleware.CanSendMessage(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanUpdateRoom tests.
func (s *MiddlewareTestSuite) TestCanUpdateRoom_AdminCanUpdate() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanUpdateRoom(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanUpdateRoom_OwnerCanUpdate() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanUpdateRoom(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanUpdateRoom_MemberCannotUpdate() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	err = s.middleware.CanUpdateRoom(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanDeleteRoom tests.
func (s *MiddlewareTestSuite) TestCanDeleteRoom_OwnerCanDelete() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanDeleteRoom(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanDeleteRoom_AdminCannotDelete() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanDeleteRoom(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanManageMembers tests.
func (s *MiddlewareTestSuite) TestCanManageMembers_AdminCanManage() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanManageMembers(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanManageMembers_OwnerCanManage() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanManageMembers(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanManageMembers_MemberCannotManage() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	err = s.middleware.CanManageMembers(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanManageRoles tests.
func (s *MiddlewareTestSuite) TestCanManageRoles_OwnerCanManage() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanManageRoles(ctx, s.actor(profileID), roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanManageRoles_AdminCannotManage() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanManageRoles(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanDeleteMessage tests.
func (s *MiddlewareTestSuite) TestCanDeleteMessage_SenderCanDelete() {
	ctx := context.Background()
	roomID := util.IDString()
	messageID := util.IDString()
	profileID := util.IDString()

	// Sender can always delete their own message (fast path)
	err := s.middleware.CanDeleteMessage(ctx, s.actor(profileID), messageID, profileID, roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanDeleteMessage_AdminCanDeleteOthers() {
	ctx := context.Background()
	roomID := util.IDString()
	messageID := util.IDString()
	adminID := util.IDString()
	senderID := util.IDString()

	// Admin can delete others' messages
	err := s.mockService.AddRoomMember(roomID, adminID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	err = s.middleware.CanDeleteMessage(ctx, s.actor(adminID), messageID, senderID, roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanDeleteMessage_OwnerCanDeleteOthers() {
	ctx := context.Background()
	roomID := util.IDString()
	messageID := util.IDString()
	ownerID := util.IDString()
	senderID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, ownerID, authz.RoleOwner)
	require.NoError(s.T(), err)

	err = s.middleware.CanDeleteMessage(ctx, s.actor(ownerID), messageID, senderID, roomID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanDeleteMessage_MemberCannotDeleteOthers() {
	ctx := context.Background()
	roomID := util.IDString()
	messageID := util.IDString()
	memberID := util.IDString()
	senderID := util.IDString()

	err := s.mockService.AddRoomMember(roomID, memberID, authz.RoleMember)
	require.NoError(s.T(), err)

	err = s.middleware.CanDeleteMessage(ctx, s.actor(memberID), messageID, senderID, roomID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanEditMessage tests.
func (s *MiddlewareTestSuite) TestCanEditMessage_SenderCanEdit() {
	ctx := context.Background()
	messageID := util.IDString()
	profileID := util.IDString()

	// Sender can always edit their own message
	err := s.middleware.CanEditMessage(ctx, s.actor(profileID), messageID, profileID)
	require.NoError(s.T(), err)
}

func (s *MiddlewareTestSuite) TestCanEditMessage_OthersCannotEdit() {
	ctx := context.Background()
	messageID := util.IDString()
	profileID := util.IDString()
	senderID := util.IDString()

	// Others cannot edit (even admin/owner)
	err := s.middleware.CanEditMessage(ctx, s.actor(profileID), messageID, senderID)
	require.Error(s.T(), err)
	require.True(s.T(), errors.Is(err, authorizer.ErrPermissionDenied))
}

// CanSendMessagesToRooms tests.
func (s *MiddlewareTestSuite) TestCanSendMessagesToRooms_BatchCheck() {
	ctx := context.Background()
	profileID := util.IDString()
	room1ID := util.IDString()
	room2ID := util.IDString()
	room3ID := util.IDString()

	// Add member to room1 and room2, but not room3
	err := s.mockService.AddRoomMember(room1ID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)
	err = s.mockService.AddRoomMember(room2ID, profileID, authz.RoleAdmin)
	require.NoError(s.T(), err)

	roomIDs := []string{room1ID, room2ID, room3ID}
	allowed, err := s.middleware.CanSendMessagesToRooms(ctx, s.actor(profileID), roomIDs)
	require.NoError(s.T(), err)

	s.True(allowed[room1ID], "Member should be able to send to room1")
	s.True(allowed[room2ID], "Admin should be able to send to room2")
	s.False(allowed[room3ID], "Non-member should not be able to send to room3")
}

func (s *MiddlewareTestSuite) TestCanSendMessagesToRooms_EmptyList() {
	ctx := context.Background()
	profileID := util.IDString()

	allowed, err := s.middleware.CanSendMessagesToRooms(ctx, s.actor(profileID), []string{})
	require.NoError(s.T(), err)
	s.Empty(allowed)
}

// AddRoomMember tests
func (s *MiddlewareTestSuite) TestAddRoomMember_CreatesMemberTuple() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.middleware.AddRoomMember(ctx, roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	// Verify tuple was created
	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: authz.RelationMember,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	}
	s.True(s.mockService.HasTuple(tuple))
}

func (s *MiddlewareTestSuite) TestAddRoomMember_CreatesOwnerTuple() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	err := s.middleware.AddRoomMember(ctx, roomID, profileID, authz.RoleOwner)
	require.NoError(s.T(), err)

	tuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: authz.RelationOwner,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	}
	s.True(s.mockService.HasTuple(tuple))
}

// RemoveRoomMember tests
func (s *MiddlewareTestSuite) TestRemoveRoomMember_RemovesAllRelations() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	// Add member
	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	// Remove member
	err = s.middleware.RemoveRoomMember(ctx, roomID, profileID)
	require.NoError(s.T(), err)

	// Verify all tuples for this member are removed
	tuples := s.mockService.GetTuples()
	for _, tuple := range tuples {
		if tuple.Object.ID == roomID && tuple.Subject.ID == profileID {
			s.Fail("Found remaining tuple for removed member")
		}
	}
}

// UpdateRoomMemberRole tests
func (s *MiddlewareTestSuite) TestUpdateRoomMemberRole_UpdatesRole() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	// Add member
	err := s.mockService.AddRoomMember(roomID, profileID, authz.RoleMember)
	require.NoError(s.T(), err)

	// Update to admin
	err = s.middleware.UpdateRoomMemberRole(ctx, roomID, profileID, authz.RoleMember, authz.RoleAdmin)
	require.NoError(s.T(), err)

	// Verify new admin tuple exists
	adminTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: authz.RelationAdmin,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	}
	s.True(s.mockService.HasTuple(adminTuple))

	// Verify old member tuple is removed
	memberTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
		Relation: authz.RelationMember,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: profileID},
	}
	s.False(s.mockService.HasTuple(memberTuple), "old member role should be removed")
}

// SetMessageSender tests
func (s *MiddlewareTestSuite) TestSetMessageSender_CreatesTuples() {
	ctx := context.Background()
	messageID := util.IDString()
	senderID := util.IDString()
	roomID := util.IDString()

	err := s.middleware.SetMessageSender(ctx, messageID, senderID, roomID)
	require.NoError(s.T(), err)

	// Verify sender tuple
	senderTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceMessage, ID: messageID},
		Relation: authz.RelationSender,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceProfile, ID: senderID},
	}
	s.True(s.mockService.HasTuple(senderTuple))

	// Verify room tuple
	roomTuple := security.RelationTuple{
		Object:   security.ObjectRef{Namespace: authz.NamespaceMessage, ID: messageID},
		Relation: authz.RelationRoom,
		Subject:  security.SubjectRef{Namespace: authz.NamespaceRoom, ID: roomID},
	}
	s.True(s.mockService.HasTuple(roomTuple))
}

// Service error handling tests
func (s *MiddlewareTestSuite) TestCanViewRoom_ServiceError() {
	ctx := context.Background()
	roomID := util.IDString()
	profileID := util.IDString()

	// Configure mock to return error
	s.mockService.CheckFunc = func(ctx context.Context, req security.CheckRequest) (security.CheckResult, error) {
		return security.CheckResult{}, errors.New("service unavailable")
	}

	err := s.middleware.CanViewRoom(ctx, s.actor(profileID), roomID)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "authorization check failed")
}
