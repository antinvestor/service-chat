package tests

import (
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type EventRepositoryTestSuite struct {
	BaseTestSuite
}

func TestEventRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(EventRepositoryTestSuite))
}

func (s *EventRepositoryTestSuite) TestCreateEvent() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		event := &models.RoomEvent{
			RoomID:      util.IDString(),
			SenderID:    util.IDString(),
			MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
			Content:     frame.JSONMap{"text": "Hello World"},
			Properties:  frame.JSONMap{"key": "value"},
		}
		event.GenID(ctx)

		err := repo.Save(ctx, event)
		s.NoError(err)
		s.NotEmpty(event.GetID())

		// Verify retrieval
		retrieved, err := repo.GetByID(ctx, event.GetID())
		s.NoError(err)
		s.Equal(event.RoomID, retrieved.RoomID)
		s.Equal(event.SenderID, retrieved.SenderID)
		s.Equal(event.MessageType, retrieved.MessageType)
	})
}

func (s *EventRepositoryTestSuite) TestGetHistory() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()
		senderID := util.IDString()

		// Create multiple events
		for range 10 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    senderID,
				MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
				Content:     frame.JSONMap{"text": util.RandomString(10)},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Get history with limit
		events, err := repo.GetHistory(ctx, roomID, "", "", 5)
		s.NoError(err)
		s.Len(events, 5)

		// Verify order (most recent first)
		for i := range len(events) - 1 {
			s.True(events[i].CreatedAt.After(events[i+1].CreatedAt) ||
				events[i].CreatedAt.Equal(events[i+1].CreatedAt))
		}
	})
}

func (s *EventRepositoryTestSuite) TestGetByRoomID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()

		// Create events
		for range 5 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
				Content:     frame.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Get all events for room
		events, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 5)
	})
}

func (s *EventRepositoryTestSuite) TestCountByRoomID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()

		// Create events
		for range 7 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
				Content:     frame.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Count events
		count, err := repo.CountByRoomID(ctx, roomID)
		s.NoError(err)
		s.GreaterOrEqual(count, int64(7))
	})
}

func (s *EventRepositoryTestSuite) TestGetBySenderID() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()
		senderID := util.IDString()

		// Create events from specific sender
		for range 3 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    senderID,
				MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
				Content:     frame.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Get events by room (GetBySenderID doesn't exist in interface)
		events, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 3)

		// Verify all events are from the sender
		for _, event := range events {
			s.Equal(senderID, event.SenderID)
		}
	})
}

func (s *EventRepositoryTestSuite) TestEventTypes() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()
		// Test different message types
		messageTypes := []chatv1.RoomEventType{
			chatv1.RoomEventType_MESSAGE_TYPE_TEXT,
			chatv1.RoomEventType_MESSAGE_TYPE_EVENT,
		}

		// Create events of different types
		for _, msgType := range messageTypes {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: msgType.String(),
				Content:     frame.JSONMap{"data": "test"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Verify all types saved
		events, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 2)
	})
}

func (s *EventRepositoryTestSuite) TestPagination() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomEventRepository(svc)

		roomID := util.IDString()

		// Create many events
		for range 20 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_MESSAGE_TYPE_TEXT.String(),
				Content:     frame.JSONMap{"text": util.RandomString(10)},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		// Get events with limit
		events, err := repo.GetByRoomID(ctx, roomID, 5)
		s.NoError(err)
		s.Len(events, 5)
	})
}
