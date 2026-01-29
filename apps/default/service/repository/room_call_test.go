package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type RoomCallRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestRoomCallRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RoomCallRepositoryTestSuite))
}

func (s *RoomCallRepositoryTestSuite) createCall(
	ctx context.Context,
	repo repository.RoomCallRepository,
	roomID, callID, status string,
) *models.RoomCall {
	t := s.T()

	call := &models.RoomCall{
		RoomID:    roomID,
		CallID:    callID,
		Status:    status,
		StartedAt: time.Now(),
		Metadata:  data.JSONMap{"type": "video"},
	}
	call.GenID(ctx)
	require.NoError(t, repo.Create(ctx, call))
	return call
}

func (s *RoomCallRepositoryTestSuite) withRepo(
	testFunc func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository),
) {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomCallRepository(ctx, dbPool, workMan)
		testFunc(t, ctx, repo)
	})
}

func (s *RoomCallRepositoryTestSuite) TestCreateAndGetByID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()
		callID := util.IDString()

		call := s.createCall(ctx, repo, roomID, callID, repository.CallStatusRinging)

		retrieved, err := repo.GetByID(ctx, call.GetID())
		require.NoError(t, err)
		s.Equal(roomID, retrieved.RoomID)
		s.Equal(callID, retrieved.CallID)
		s.Equal(repository.CallStatusRinging, retrieved.Status)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetByCallID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()
		callID := util.IDString()

		s.createCall(ctx, repo, roomID, callID, repository.CallStatusActive)

		retrieved, err := repo.GetByCallID(ctx, callID)
		require.NoError(t, err)
		s.Equal(roomID, retrieved.RoomID)
		s.Equal(callID, retrieved.CallID)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetByRoomID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()

		// Create 3 calls in this room
		for range 3 {
			s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusEnded)
		}

		// Create 1 call in different room
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)

		calls, err := repo.GetByRoomID(ctx, roomID, 10)
		require.NoError(t, err)
		s.Len(calls, 3)

		// With limit
		calls, err = repo.GetByRoomID(ctx, roomID, 2)
		require.NoError(t, err)
		s.Len(calls, 2)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetActiveCallByRoomID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()

		// Create ended call
		s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusEnded)

		// Create active call
		activeCall := s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusActive)

		retrieved, err := repo.GetActiveCallByRoomID(ctx, roomID)
		require.NoError(t, err)
		s.Equal(activeCall.CallID, retrieved.CallID)
		s.Equal(repository.CallStatusActive, retrieved.Status)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetActiveCallByRoomID_Ringing() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()

		// Ringing counts as active
		ringingCall := s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusRinging)

		retrieved, err := repo.GetActiveCallByRoomID(ctx, roomID)
		require.NoError(t, err)
		s.Equal(ringingCall.CallID, retrieved.CallID)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetByStatus() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		// Create calls with different statuses
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusRinging)
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusRinging)
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusEnded)

		ringingCalls, err := repo.GetByStatus(ctx, repository.CallStatusRinging, 10)
		require.NoError(t, err)
		s.Len(ringingCalls, 2)

		activeCalls, err := repo.GetByStatus(ctx, repository.CallStatusActive, 10)
		require.NoError(t, err)
		s.Len(activeCalls, 1)

		endedCalls, err := repo.GetByStatus(ctx, repository.CallStatusEnded, 10)
		require.NoError(t, err)
		s.Len(endedCalls, 1)
	})
}

func (s *RoomCallRepositoryTestSuite) TestUpdateStatus() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		call := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusRinging)

		err := repo.UpdateStatus(ctx, call.GetID(), repository.CallStatusActive)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, call.GetID())
		require.NoError(t, err)
		s.Equal(repository.CallStatusActive, retrieved.Status)
	})
}

func (s *RoomCallRepositoryTestSuite) TestUpdateSFUNode() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		call := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)

		sfuNodeID := "sfu-node-001"
		err := repo.UpdateSFUNode(ctx, call.GetID(), sfuNodeID)
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, call.GetID())
		require.NoError(t, err)
		s.Equal(sfuNodeID, retrieved.SFUNodeID)
	})
}

func (s *RoomCallRepositoryTestSuite) TestEndCall() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		call := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)

		err := repo.EndCall(ctx, call.GetID())
		require.NoError(t, err)

		retrieved, err := repo.GetByID(ctx, call.GetID())
		require.NoError(t, err)
		s.Equal(repository.CallStatusEnded, retrieved.Status)
		s.False(retrieved.EndedAt.IsZero(), "EndedAt should be set")
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetTimedOutCalls() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		// Create a ringing call with old start time
		oldCall := &models.RoomCall{
			RoomID:    util.IDString(),
			CallID:    util.IDString(),
			Status:    repository.CallStatusRinging,
			StartedAt: time.Now().Add(-10 * time.Minute),
		}
		oldCall.GenID(ctx)
		require.NoError(t, repo.Create(ctx, oldCall))

		// Create a recent ringing call
		s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusRinging)

		// Get calls timed out after 5 minutes
		timedOut, err := repo.GetTimedOutCalls(ctx, 5*time.Minute)
		require.NoError(t, err)
		s.Len(timedOut, 1)
		s.Equal(oldCall.CallID, timedOut[0].CallID)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetCallDuration() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		// Create a call with known start time and end it
		call := &models.RoomCall{
			RoomID:    util.IDString(),
			CallID:    util.IDString(),
			Status:    repository.CallStatusActive,
			StartedAt: time.Now().Add(-5 * time.Minute),
		}
		call.GenID(ctx)
		require.NoError(t, repo.Create(ctx, call))

		// For active call, duration is calculated from now
		duration, err := repo.GetCallDuration(ctx, call.GetID())
		require.NoError(t, err)
		s.GreaterOrEqual(duration, 5*time.Minute-15*time.Second)

		// End the call
		require.NoError(t, repo.EndCall(ctx, call.GetID()))

		// Duration should now be from start to end
		duration, err = repo.GetCallDuration(ctx, call.GetID())
		require.NoError(t, err)
		s.GreaterOrEqual(duration, 5*time.Minute-15*time.Second)
	})
}

func (s *RoomCallRepositoryTestSuite) TestCountActiveCallsByRoomID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		roomID := util.IDString()

		// Create active and ringing calls (both count as active)
		s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusActive)
		s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusRinging)
		// Ended call should not count
		s.createCall(ctx, repo, roomID, util.IDString(), repository.CallStatusEnded)

		count, err := repo.CountActiveCallsByRoomID(ctx, roomID)
		require.NoError(t, err)
		s.Equal(int64(2), count)
	})
}

func (s *RoomCallRepositoryTestSuite) TestGetCallsBySFUNode() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomCallRepository) {
		sfuNodeID := "sfu-node-test"

		// Create active calls on this SFU node
		call1 := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)
		require.NoError(t, repo.UpdateSFUNode(ctx, call1.GetID(), sfuNodeID))

		call2 := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusRinging)
		require.NoError(t, repo.UpdateSFUNode(ctx, call2.GetID(), sfuNodeID))

		// Create ended call on this SFU node (should not be returned)
		call3 := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)
		require.NoError(t, repo.UpdateSFUNode(ctx, call3.GetID(), sfuNodeID))
		require.NoError(t, repo.EndCall(ctx, call3.GetID()))

		// Create active call on different SFU node
		call4 := s.createCall(ctx, repo, util.IDString(), util.IDString(), repository.CallStatusActive)
		require.NoError(t, repo.UpdateSFUNode(ctx, call4.GetID(), "other-sfu"))

		calls, err := repo.GetCallsBySFUNode(ctx, sfuNodeID)
		require.NoError(t, err)
		s.Len(calls, 2)
	})
}
