package repository_test

import (
	"context"
	"testing"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
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

type EventRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestEventRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(EventRepositoryTestSuite))
}

func (s *EventRepositoryTestSuite) withRepo(
	testFunc func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository),
) {
	frametests.WithTestDependencies(s.T(), nil, func(t *testing.T, dep *definition.DependencyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(ctx, dbPool, workMan)
		testFunc(t, ctx, repo)
	})
}

func (s *EventRepositoryTestSuite) TestCreateEvent() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		event := &models.RoomEvent{
			RoomID:     util.IDString(),
			SenderID:   util.IDString(),
			EventType:  int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
			Content:    data.JSONMap{"text": "Hello World"},
			Properties: data.JSONMap{"key": "value"},
		}
		event.GenID(ctx)

		err := repo.Create(ctx, event)
		require.NoError(t, err)
		s.NotEmpty(event.GetID())

		retrieved, err := repo.GetByID(ctx, event.GetID())
		require.NoError(t, err)
		s.Equal(event.RoomID, retrieved.RoomID)
		s.Equal(event.SenderID, retrieved.SenderID)
		s.Equal(event.EventType, retrieved.EventType)
	})
}

func (s *EventRepositoryTestSuite) TestGetHistory() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()
		senderID := util.IDString()

		for range 10 {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  senderID,
				EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
				Content:   data.JSONMap{"text": util.RandomAlphaNumericString(10)},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
		}

		events, err := repo.GetHistory(ctx, roomID, "", "", 5)
		require.NoError(t, err)
		s.Len(events, 5)

		for i := range len(events) - 1 {
			s.True(events[i].CreatedAt.After(events[i+1].CreatedAt) ||
				events[i].CreatedAt.Equal(events[i+1].CreatedAt))
		}
	})
}

func (s *EventRepositoryTestSuite) TestGetByRoomID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()

		for range 5 {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  util.IDString(),
				EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
				Content:   data.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
		}

		events, err := repo.GetByRoomID(ctx, roomID, 10)
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), 5)
	})
}

func (s *EventRepositoryTestSuite) TestCountByRoomID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()

		for range 7 {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  util.IDString(),
				EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
				Content:   data.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
		}

		count, err := repo.CountByRoomID(ctx, roomID)
		require.NoError(t, err)
		s.GreaterOrEqual(count, int64(7))
	})
}

func (s *EventRepositoryTestSuite) TestGetByEventID() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()
		senderID := util.IDString()

		event := &models.RoomEvent{
			RoomID:    roomID,
			SenderID:  senderID,
			EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
			Content:   data.JSONMap{"text": "Message"},
		}
		event.GenID(ctx)
		require.NoError(t, repo.Create(ctx, event))

		retrieved, err := repo.GetByEventID(ctx, roomID, event.GetID())
		require.NoError(t, err)
		s.NotNil(retrieved)
		s.Equal(senderID, retrieved.SenderID)
	})
}

func (s *EventRepositoryTestSuite) TestEventTypes() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()
		messageTypes := []chatv1.RoomEventType{
			chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE,
			chatv1.RoomEventType_ROOM_EVENT_TYPE_EVENT,
		}

		for _, msgType := range messageTypes {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  util.IDString(),
				EventType: int32(msgType.Number()),
				Content:   data.JSONMap{"data": "test"},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
		}

		events, err := repo.GetByRoomID(ctx, roomID, 10)
		require.NoError(t, err)
		s.GreaterOrEqual(len(events), 2)
	})
}

func (s *EventRepositoryTestSuite) TestExistsByIDs() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()

		// Create 3 events
		var eventIDs []string
		for range 3 {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  util.IDString(),
				EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
				Content:   data.JSONMap{"text": util.RandomAlphaNumericString(10)},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
			eventIDs = append(eventIDs, event.GetID())
		}

		nonExistentID := util.IDString()
		checkIDs := append([]string{}, eventIDs...)
		checkIDs = append(checkIDs, nonExistentID)

		existsMap, err := repo.ExistsByIDs(ctx, checkIDs)
		require.NoError(t, err)

		// Existing IDs should return true
		for _, id := range eventIDs {
			s.True(existsMap[id], "event %s should exist", id)
		}
		// Non-existent ID should return false
		s.False(existsMap[nonExistentID], "non-existent event should not exist")
	})
}

func (s *EventRepositoryTestSuite) TestPagination() {
	s.withRepo(func(t *testing.T, ctx context.Context, repo repository.RoomEventRepository) {
		roomID := util.IDString()

		for range 20 {
			event := &models.RoomEvent{
				RoomID:    roomID,
				SenderID:  util.IDString(),
				EventType: int32(chatv1.RoomEventType_ROOM_EVENT_TYPE_MESSAGE.Number()),
				Content:   data.JSONMap{"text": util.RandomAlphaNumericString(10)},
			}
			event.GenID(ctx)
			require.NoError(t, repo.Create(ctx, event))
		}

		page1, err := repo.GetByRoomID(ctx, roomID, 10)
		require.NoError(t, err)
		s.Len(page1, 10)

		page2, err := repo.GetHistory(ctx, roomID, page1[len(page1)-1].GetID(), "", 10)
		require.NoError(t, err)
		s.GreaterOrEqual(len(page2), 1)
	})
}
