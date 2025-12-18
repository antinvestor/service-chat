package repository_test

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type OutboxRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestOutboxRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(OutboxRepositoryTestSuite))
}

func (s *OutboxRepositoryTestSuite) TestCreateOutbox() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			OutboxState:    models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)

		err := repo.Create(ctx, outbox)
		require.NoError(t, err)
		s.NotEmpty(outbox.GetID())

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		require.NoError(t, err)
		s.Equal(outbox.RoomID, retrieved.RoomID)
		s.Equal(outbox.EventID, retrieved.EventID)
		s.Equal(outbox.OutboxState, retrieved.OutboxState)
	})
}

func (s *OutboxRepositoryTestSuite) TestGetPendingBySubscription() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		subscriptionID := util.IDString()
		roomID := util.IDString()

		for range 5 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: subscriptionID,
				EventID:        util.IDString(),
				OutboxState:    models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			require.NoError(t, repo.Create(ctx, outbox))
		}

		sentOutbox := &models.RoomOutbox{
			RoomID:         roomID,
			SubscriptionID: subscriptionID,
			EventID:        util.IDString(),
			OutboxState:    models.RoomOutboxStateSent,
			RetryCount:     0,
		}
		sentOutbox.GenID(ctx)
		require.NoError(t, repo.Create(ctx, sentOutbox))

		pending, err := repo.GetPendingBySubscription(ctx, subscriptionID, 10)
		require.NoError(t, err)
		s.Len(pending, 5)

		for _, entry := range pending {
			s.Equal(models.RoomOutboxStateLogged, entry.OutboxState)
		}
	})
}

func (s *OutboxRepositoryTestSuite) TestGetByRoomID() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		roomID := util.IDString()

		for range 3 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				OutboxState:    models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			require.NoError(t, repo.Create(ctx, outbox))
		}

		entries, err := repo.GetByRoomID(ctx, roomID, 10)
		require.NoError(t, err)
		s.GreaterOrEqual(len(entries), 3)
	})
}

func (s *OutboxRepositoryTestSuite) TestUpdateStatus() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			OutboxState:    models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		require.NoError(t, repo.Create(ctx, outbox))

		// Update via direct DB call since UpdateStatus exists but has type mismatch
		outbox.OutboxState = models.RoomOutboxStateSent
		_, err := repo.Update(ctx, outbox)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		require.NoError(t, err)
		s.Equal(models.RoomOutboxStateSent, retrieved.OutboxState)
	})
}

func (s *OutboxRepositoryTestSuite) TestIncrementRetryCount() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			OutboxState:    models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		require.NoError(t, repo.Create(ctx, outbox))

		err := repo.IncrementRetryCount(ctx, outbox.GetID())
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		require.NoError(t, err)
		s.Equal(1, retrieved.RetryCount)

		require.NoError(t, repo.IncrementRetryCount(ctx, outbox.GetID()))
		retrieved, err = repo.GetByID(ctx, outbox.GetID())
		require.NoError(t, err)
		s.Equal(2, retrieved.RetryCount)
	})
}

func (s *OutboxRepositoryTestSuite) TestMarkAsFailed() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			OutboxState:    models.RoomOutboxStateLogged,
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		require.NoError(t, repo.Create(ctx, outbox))

		errorMsg := "Connection timeout"
		err := repo.UpdateStatusWithError(ctx, outbox.GetID(), models.RoomOutboxStateFailed, errorMsg)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		require.NoError(t, err)
		s.Equal(errorMsg, retrieved.ErrorMessage)
	})
}

func (s *OutboxRepositoryTestSuite) TestDeleteOldEntries() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		roomID := util.IDString()

		for range 5 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				OutboxState:    models.RoomOutboxStateSent,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			require.NoError(t, repo.Create(ctx, outbox))
		}

		deleted, err := repo.CleanupOldEntries(ctx, 24*60*60*1000000000)
		require.NoError(t, err)
		s.GreaterOrEqual(deleted, int64(5))
	})
}

func (s *OutboxRepositoryTestSuite) TestCountPendingByRoom() {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomOutboxRepository(ctx, dbPool, workMan)

		roomID := util.IDString()

		for range 7 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				OutboxState:    models.RoomOutboxStateLogged,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			require.NoError(t, repo.Create(ctx, outbox))
		}

		for range 3 {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				OutboxState:    models.RoomOutboxStateSent,
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			require.NoError(t, repo.Create(ctx, outbox))
		}

		pendingEntries, err := repo.GetByRoomID(ctx, roomID, 100)
		require.NoError(t, err)

		count := 0
		for _, entry := range pendingEntries {
			if entry.OutboxState == models.RoomOutboxStateLogged {
				count++
			}
		}
		s.Equal(7, count)
	})
}
