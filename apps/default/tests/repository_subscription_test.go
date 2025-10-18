package tests

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type SubscriptionRepositoryTestSuite struct {
	BaseTestSuite
}

func TestSubscriptionRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionRepositoryTestSuite))
}

func (s *SubscriptionRepositoryTestSuite) TestCreateSubscription() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		sub := &models.RoomSubscription{
			RoomID:               util.IDString(),
			ProfileID:            util.IDString(),
			Role:                 repository.RoleMember,
			IsActive:             true,
			NotificationsEnabled: true,
			Properties:           frame.JSONMap{"test": "data"},
		}
		sub.GenID(ctx)

		err := repo.Save(ctx, sub)
		s.NoError(err)
		s.NotEmpty(sub.GetID())

		// Verify retrieval
		retrieved, err := repo.GetByID(ctx, sub.GetID())
		s.NoError(err)
		s.Equal(sub.RoomID, retrieved.RoomID)
		s.Equal(sub.ProfileID, retrieved.ProfileID)
		s.Equal(sub.Role, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByRoomAndProfile() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()
		profileID := util.IDString()

		sub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: profileID,
			Role:      repository.RoleAdmin,
			IsActive:  true,
		}
		sub.GenID(ctx)
		s.NoError(repo.Save(ctx, sub))

		// Retrieve by room and profile
		retrieved, err := repo.GetByRoomAndProfile(ctx, roomID, profileID)
		s.NoError(err)
		s.Equal(sub.GetID(), retrieved.GetID())
		s.Equal(repository.RoleAdmin, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetActiveByRoomAndProfile() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()
		profileID := util.IDString()

		// Create active subscription
		activeSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: profileID,
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		activeSub.GenID(ctx)
		s.NoError(repo.Save(ctx, activeSub))

		// Retrieve active subscription
		retrieved, err := repo.GetActiveByRoomAndProfile(ctx, roomID, profileID)
		s.NoError(err)
		s.True(retrieved.IsActive)

		// Deactivate
		s.NoError(repo.Deactivate(ctx, activeSub.GetID()))

		// Should not find inactive subscription
		_, err = repo.GetActiveByRoomAndProfile(ctx, roomID, profileID)
		s.Error(err)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByRoomID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()

		// Create multiple subscriptions
		for i := 0; i < 3; i++ {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: util.IDString(),
				Role:      repository.RoleMember,
				IsActive:  i < 2, // First two active, last one inactive
			}
			sub.GenID(ctx)
			s.NoError(repo.Save(ctx, sub))
		}

		// Get all subscriptions
		allSubs, err := repo.GetByRoomID(ctx, roomID, false)
		s.NoError(err)
		s.Len(allSubs, 3)

		// Get only active subscriptions
		activeSubs, err := repo.GetByRoomID(ctx, roomID, true)
		s.NoError(err)
		s.Len(activeSubs, 2)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByProfileID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		profileID := util.IDString()

		// Create subscriptions for different rooms
		for i := 0; i < 3; i++ {
			sub := &models.RoomSubscription{
				RoomID:    util.IDString(),
				ProfileID: profileID,
				Role:      repository.RoleMember,
				IsActive:  true,
			}
			sub.GenID(ctx)
			s.NoError(repo.Save(ctx, sub))
		}

		// Get all subscriptions for profile
		subs, err := repo.GetByProfileID(ctx, profileID, true)
		s.NoError(err)
		s.Len(subs, 3)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestUpdateRole() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		sub := &models.RoomSubscription{
			RoomID:    util.IDString(),
			ProfileID: util.IDString(),
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		sub.GenID(ctx)
		s.NoError(repo.Save(ctx, sub))

		// Update role
		err := repo.UpdateRole(ctx, sub.GetID(), repository.RoleAdmin)
		s.NoError(err)

		// Verify update
		retrieved, err := repo.GetByID(ctx, sub.GetID())
		s.NoError(err)
		s.Equal(repository.RoleAdmin, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestHasPermission() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()
		adminID := util.IDString()
		memberID := util.IDString()

		// Create admin subscription
		adminSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: adminID,
			Role:      repository.RoleAdmin,
			IsActive:  true,
		}
		adminSub.GenID(ctx)
		s.NoError(repo.Save(ctx, adminSub))

		// Create member subscription
		memberSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: memberID,
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		memberSub.GenID(ctx)
		s.NoError(repo.Save(ctx, memberSub))

		// Admin should have member permission
		hasPermission, err := repo.HasPermission(ctx, roomID, adminID, repository.RoleMember)
		s.NoError(err)
		s.True(hasPermission)

		// Member should not have admin permission
		hasPermission, err = repo.HasPermission(ctx, roomID, memberID, repository.RoleAdmin)
		s.NoError(err)
		s.False(hasPermission)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestCountActiveMembers() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()

		// Create 5 subscriptions, 3 active
		for i := 0; i < 5; i++ {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: util.IDString(),
				Role:      repository.RoleMember,
				IsActive:  i < 3,
			}
			sub.GenID(ctx)
			s.NoError(repo.Save(ctx, sub))
		}

		count, err := repo.CountActiveMembers(ctx, roomID)
		s.NoError(err)
		s.Equal(int64(3), count)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestBulkCreate() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()
		subs := []*models.RoomSubscription{}

		for i := 0; i < 5; i++ {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: util.IDString(),
				Role:      repository.RoleMember,
				IsActive:  true,
			}
			sub.GenID(ctx)
			subs = append(subs, sub)
		}

		err := repo.BulkCreate(ctx, subs)
		s.NoError(err)

		// Verify all created
		retrieved, err := repo.GetByRoomID(ctx, roomID, true)
		s.NoError(err)
		s.Len(retrieved, 5)
	})
}
