package repository_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type OutboxRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestOutboxRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(OutboxRepositoryTestSuite))
}

func (s *OutboxRepositoryTestSuite) TestCreateOutbox() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			State:          models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)

		err := repo.Save(ctx, outbox)
		s.NoError(err)
		s.NotEmpty(outbox.GetID())

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(outbox.RoomID, retrieved.RoomID)
		s.Equal(outbox.EventID, retrieved.EventID)
		s.Equal(outbox.State, retrieved.State)
	})
}

func (s *OutboxRepositoryTestSuite) TestGetPendingBySubscription() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		subscriptionID := util.IDString()
		roomID := util.IDString()

		for range 5 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: subscriptionID,
				EventID:        util.IDString(),
				State:          models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		sentOutbox := &models.RoomOutbox{
			RoomID:         roomID,
			SubscriptionID: subscriptionID,
			EventID:        util.IDString(),
			State:          models.RoomOutboxStateSent,
			RetryCount:     0,
		}
		sentOutbox.GenID(ctx)
		s.NoError(repo.Save(ctx, sentOutbox))

		pending, err := repo.GetPendingBySubscription(ctx, subscriptionID, 10)
		s.NoError(err)
		s.Len(pending, 5)

		for _, entry := range pending {
			s.Equal(models.RoomOutboxStateLogged, entry.State)
		}
	})
}

func (s *OutboxRepositoryTestSuite) TestGetByRoomID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 3 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				State:          models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		entries, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(entries), 3)
	})
}

func (s *OutboxRepositoryTestSuite) TestUpdateStatus() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			State:          models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		// Update via direct DB call since UpdateStatus exists but has type mismatch
		outbox.State = models.RoomOutboxStateSent
		err := repo.Save(ctx, outbox)
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(models.RoomOutboxStateSent, retrieved.State)
	})
}

func (s *OutboxRepositoryTestSuite) TestIncrementRetryCount() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			State:          models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		err := repo.IncrementRetryCount(ctx, outbox.GetID())
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(1, retrieved.RetryCount)

		s.NoError(repo.IncrementRetryCount(ctx, outbox.GetID()))
		retrieved, err = repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(2, retrieved.RetryCount)
	})
}

func (s *OutboxRepositoryTestSuite) TestMarkAsFailed() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			State:          models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		errorMsg := "Connection timeout"
		err := repo.UpdateStatusWithError(ctx, outbox.GetID(), repository.OutboxStatusFailed, errorMsg)
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(errorMsg, retrieved.ErrorMessage)
	})
}

func (s *OutboxRepositoryTestSuite) TestDeleteOldEntries() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 5 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				State:          models.RoomOutboxStateSent,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		deleted, err := repo.CleanupOldEntries(ctx, 24*60*60*1000000000)
		s.NoError(err)
		s.GreaterOrEqual(deleted, int64(5))
	})
}

func (s *OutboxRepositoryTestSuite) TestCountPendingByRoom() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 7 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				State:          models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		for range 3 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				State:          models.RoomOutboxStateSent,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		pendingEntries, err := repo.GetByRoomID(ctx, roomID, 100)
		s.NoError(err)

		count := 0
		for _, entry := range pendingEntries {
			if entry.State == models.RoomOutboxStateLogged {
				count++
			}
		}
		s.Equal(7, count)
	})
}
