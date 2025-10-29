package repository_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type SubscriptionRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestSubscriptionRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionRepositoryTestSuite))
}

func (s *SubscriptionRepositoryTestSuite) TestCreateSubscription() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		sub := &models.RoomSubscription{
			RoomID:               util.IDString(),
			ProfileID:            util.IDString(),
			Role:                 repository.RoleMember,
			IsActive:             true,
			NotificationsEnabled: true,
			Properties:           data.JSONMap{"test": "data"},
		}
		sub.GenID(ctx)

		err := repo.Save(ctx, sub)
		s.NoError(err)
		s.NotEmpty(sub.GetID())

		retrieved, err := repo.GetByID(ctx, sub.GetID())
		s.NoError(err)
		s.Equal(sub.RoomID, retrieved.RoomID)
		s.Equal(sub.ProfileID, retrieved.ProfileID)
		s.Equal(sub.Role, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByRoomAndProfile() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

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

		retrieved, err := repo.GetByRoomAndProfile(ctx, roomID, profileID)
		s.NoError(err)
		s.Equal(sub.GetID(), retrieved.GetID())
		s.Equal(repository.RoleAdmin, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetActiveByRoomAndProfile() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		roomID := util.IDString()
		profileID := util.IDString()

		activeSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: profileID,
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		activeSub.GenID(ctx)
		s.NoError(repo.Save(ctx, activeSub))

		retrieved, err := repo.GetActiveByRoomAndProfile(ctx, roomID, profileID)
		s.NoError(err)
		s.Equal(activeSub.GetID(), retrieved.GetID())

		s.NoError(repo.Deactivate(ctx, activeSub.GetID()))

		_, err = repo.GetActiveByRoomAndProfile(ctx, roomID, profileID)
		s.Error(err)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByRoomID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		roomID := util.IDString()

		for i := range 3 {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: util.IDString(),
				Role:      repository.RoleMember,
				IsActive:  i < 2,
			}
			sub.GenID(ctx)
			s.NoError(repo.Save(ctx, sub))
		}

		allSubs, err := repo.GetByRoomID(ctx, roomID, false)
		s.NoError(err)
		s.Len(allSubs, 3)

		activeSubs, err := repo.GetByRoomID(ctx, roomID, true)
		s.NoError(err)
		s.Len(activeSubs, 2)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestGetByProfileID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		profileID := util.IDString()

		for range 3 {
			sub := &models.RoomSubscription{
				RoomID:    util.IDString(),
				ProfileID: profileID,
				Role:      repository.RoleMember,
				IsActive:  true,
			}
			sub.GenID(ctx)
			s.NoError(repo.Save(ctx, sub))
		}

		subs, err := repo.GetByProfileID(ctx, profileID, true)
		s.NoError(err)
		s.Len(subs, 3)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestUpdateRole() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		sub := &models.RoomSubscription{
			RoomID:    util.IDString(),
			ProfileID: util.IDString(),
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		sub.GenID(ctx)
		s.NoError(repo.Save(ctx, sub))

		err := repo.UpdateRole(ctx, sub.GetID(), repository.RoleAdmin)
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, sub.GetID())
		s.NoError(err)
		s.Equal(repository.RoleAdmin, retrieved.Role)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestHasPermission() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		roomID := util.IDString()
		adminID := util.IDString()
		memberID := util.IDString()

		adminSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: adminID,
			Role:      repository.RoleAdmin,
			IsActive:  true,
		}
		adminSub.GenID(ctx)
		s.NoError(repo.Save(ctx, adminSub))

		memberSub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: memberID,
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		memberSub.GenID(ctx)
		s.NoError(repo.Save(ctx, memberSub))

		hasPermission, err := repo.HasPermission(ctx, roomID, adminID, repository.RoleAdmin)
		s.NoError(err)
		s.True(hasPermission)

		hasPermission, err = repo.HasPermission(ctx, roomID, memberID, repository.RoleAdmin)
		s.NoError(err)
		s.False(hasPermission)
	})
}

func (s *SubscriptionRepositoryTestSuite) TestCountActiveMembers() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		roomID := util.IDString()

		for i := range 5 {
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
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomSubscriptionRepository(dbPool, workMan)

		roomID := util.IDString()
		subs := []*models.RoomSubscription{}

		for range 5 {
			sub := &models.RoomSubscription{
				RoomID:    roomID,
				ProfileID: util.IDString(),
				Role:      repository.RoleMember,
				IsActive:  true,
			}
			sub.GenID(ctx)
			subs = append(subs, sub)
		}

		for _, sub := range subs {
			s.NoError(repo.Save(ctx, sub))
		}

		retrieved, err := repo.GetByRoomID(ctx, roomID, true)
		s.NoError(err)
		s.Len(retrieved, 5)
	})
}
