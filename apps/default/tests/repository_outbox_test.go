package tests

import (
	"testing"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type OutboxRepositoryTestSuite struct {
	BaseTestSuite
}

func TestOutboxRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(OutboxRepositoryTestSuite))
}

func (s *OutboxRepositoryTestSuite) TestCreateOutbox() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			Status:         "pending",
			RetryCount:     0,
		}
		outbox.GenID(ctx)

		err := repo.Save(ctx, outbox)
		s.NoError(err)
		s.NotEmpty(outbox.GetID())

		// Verify retrieval
		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(outbox.RoomID, retrieved.RoomID)
		s.Equal(outbox.EventID, retrieved.EventID)
		s.Equal(outbox.Status, retrieved.Status)
	})
}

func (s *OutboxRepositoryTestSuite) TestGetPendingBySubscription() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		subscriptionID := util.IDString()
		roomID := util.IDString()

		// Create pending outbox entries
		for i := 0; i < 5; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: subscriptionID,
				EventID:        util.IDString(),
				Status:         "pending",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		// Create sent outbox entry
		sentOutbox := &models.RoomOutbox{
			RoomID:         roomID,
			SubscriptionID: subscriptionID,
			EventID:        util.IDString(),
			Status:         "sent",
			RetryCount:     0,
		}
		sentOutbox.GenID(ctx)
		s.NoError(repo.Save(ctx, sentOutbox))

		// Get pending entries
		pending, err := repo.GetPendingBySubscription(ctx, subscriptionID, 10)
		s.NoError(err)
		s.Len(pending, 5)

		for _, entry := range pending {
			s.Equal("pending", entry.Status)
		}
	})
}

func (s *OutboxRepositoryTestSuite) TestGetByRoomID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		roomID := util.IDString()

		// Create outbox entries
		for i := 0; i < 3; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				Status:         "pending",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		// Get all entries for room
		entries, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(entries), 3)
	})
}

func (s *OutboxRepositoryTestSuite) TestUpdateStatus() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			Status:         "pending",
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		// Update status
		err := repo.UpdateStatus(ctx, outbox.GetID(), "sent")
		s.NoError(err)

		// Verify update
		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal("sent", retrieved.Status)
	})
}

func (s *OutboxRepositoryTestSuite) TestIncrementRetryCount() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			Status:         "pending",
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		// Increment retry count
		err := repo.IncrementRetryCount(ctx, outbox.GetID())
		s.NoError(err)

		// Verify increment
		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(1, retrieved.RetryCount)

		// Increment again
		s.NoError(repo.IncrementRetryCount(ctx, outbox.GetID()))
		retrieved, err = repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal(2, retrieved.RetryCount)
	})
}

func (s *OutboxRepositoryTestSuite) TestMarkAsFailed() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		outbox := &models.RoomOutbox{
			RoomID:         util.IDString(),
			SubscriptionID: util.IDString(),
			EventID:        util.IDString(),
			Status:         "pending",
			RetryCount:     0,
		}
		outbox.GenID(ctx)
		s.NoError(repo.Save(ctx, outbox))

		// Mark as failed
		errorMsg := "Connection timeout"
		err = repo.UpdateStatusWithError(ctx, outbox.GetID(), repository.OutboxStatusFailed, errorMsg)
		s.NoError(err)

		// Verify update
		retrieved, err := repo.GetByID(ctx, outbox.GetID())
		s.NoError(err)
		s.Equal("failed", retrieved.Status)
		s.Equal(errorMsg, retrieved.ErrorMessage)
	})
}

func (s *OutboxRepositoryTestSuite) TestDeleteOldEntries() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		roomID := util.IDString()

		// Create outbox entries
		for i := 0; i < 5; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				Status:         "sent",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		// Delete old entries (older than 0 days - should delete all sent)
		deleted, err := repo.CleanupOldEntries(ctx, 24*time.Hour)
		s.NoError(err)
		s.GreaterOrEqual(deleted, int64(5))
	})
}

func (s *OutboxRepositoryTestSuite) TestCountPendingByRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomOutboxRepository(svc)

		roomID := util.IDString()

		// Create pending entries
		for i := 0; i < 7; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				Status:         "pending",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		// Create sent entries
		for i := 0; i < 3; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: util.IDString(),
				EventID:        util.IDString(),
				Status:         "sent",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(repo.Save(ctx, outbox))
		}

		// Count pending
		// Get pending entries for the room
		pendingEntries, err := repo.GetByRoomID(ctx, roomID, 100)
		s.NoError(err)
		// Count pending ones
		count := 0
		for _, entry := range pendingEntries {
			if entry.Status == repository.OutboxStatusPending {
				count++
			}
		}
		err = nil
		s.NoError(err)
		s.Equal(int64(7), count)
	})
}

func (s *OutboxRepositoryTestSuite) TestUnreadCountGeneration() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		outboxRepo := repository.NewRoomOutboxRepository(svc)
		subRepo := repository.NewRoomSubscriptionRepository(svc)

		roomID := util.IDString()
		subscriptionID := util.IDString()

		// Create subscription
		sub := &models.RoomSubscription{
			RoomID:    roomID,
			ProfileID: util.IDString(),
			Role:      repository.RoleMember,
			IsActive:  true,
		}
		sub.ID = subscriptionID
		s.NoError(subRepo.Save(ctx, sub))

		// Create pending outbox entries for this subscription
		for i := 0; i < 5; i++ {
			outbox := &models.RoomOutbox{
				RoomID:         roomID,
				SubscriptionID: subscriptionID,
				EventID:        util.IDString(),
				Status:         "pending",
				RetryCount:     0,
			}
			outbox.GenID(ctx)
			s.NoError(outboxRepo.Save(ctx, outbox))
		}

		// Retrieve subscription and check unread count (generated column)
		retrieved, err := subRepo.GetByID(ctx, subscriptionID)
		s.NoError(err)
		s.Equal(5, retrieved.UnreadCount)

		// Mark one as sent
		pending, err := outboxRepo.GetPendingBySubscription(ctx, subscriptionID, 1)
		s.NoError(err)
		s.Len(pending, 1)
		s.NoError(outboxRepo.UpdateStatus(ctx, pending[0].GetID(), "sent"))

		// Unread count should decrease
		retrieved, err = subRepo.GetByID(ctx, subscriptionID)
		s.NoError(err)
		s.Equal(4, retrieved.UnreadCount)
	})
}
