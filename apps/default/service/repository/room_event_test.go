package repository_test

import (
	"testing"

	chatv1 "github.com/antinvestor/apis/go/chat/v1"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	"github.com/antinvestor/service-chat/apps/default/tests"
	"github.com/pitabwire/frame/data"
	"github.com/pitabwire/frame/frametests"
	"github.com/pitabwire/frame/frametests/definition"
	"github.com/pitabwire/util"
	"github.com/stretchr/testify/suite"
)

type EventRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestEventRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(EventRepositoryTestSuite))
}

func (s *EventRepositoryTestSuite) TestCreateEvent() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		event := &models.RoomEvent{
			RoomID:      util.IDString(),
			SenderID:    util.IDString(),
			MessageType: chatv1.RoomEventType_TEXT.String(),
			Content:     data.JSONMap{"text": "Hello World"},
			Properties:  data.JSONMap{"key": "value"},
		}
		event.GenID(ctx)

		err := repo.Save(ctx, event)
		s.NoError(err)
		s.NotEmpty(event.GetID())

		retrieved, err := repo.GetByID(ctx, event.GetID())
		s.NoError(err)
		s.Equal(event.RoomID, retrieved.RoomID)
		s.Equal(event.SenderID, retrieved.SenderID)
		s.Equal(event.MessageType, retrieved.MessageType)
	})
}

func (s *EventRepositoryTestSuite) TestGetHistory() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()
		senderID := util.IDString()

		for range 10 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    senderID,
				MessageType: chatv1.RoomEventType_TEXT.String(),
				Content:     data.JSONMap{"text": util.RandomString(10)},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		events, err := repo.GetHistory(ctx, roomID, "", "", 5)
		s.NoError(err)
		s.Len(events, 5)

		for i := range len(events) - 1 {
			s.True(events[i].CreatedAt.After(events[i+1].CreatedAt) ||
				events[i].CreatedAt.Equal(events[i+1].CreatedAt))
		}
	})
}

func (s *EventRepositoryTestSuite) TestGetByRoomID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 5 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_TEXT.String(),
				Content:     data.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		events, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 5)
	})
}

func (s *EventRepositoryTestSuite) TestCountByRoomID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 7 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_TEXT.String(),
				Content:     data.JSONMap{"text": "Message"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		count, err := repo.CountByRoomID(ctx, roomID)
		s.NoError(err)
		s.GreaterOrEqual(count, int64(7))
	})
}

func (s *EventRepositoryTestSuite) TestGetByEventID() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()
		senderID := util.IDString()

		event := &models.RoomEvent{
			RoomID:      roomID,
			SenderID:    senderID,
			MessageType: chatv1.RoomEventType_TEXT.String(),
			Content:     data.JSONMap{"text": "Message"},
		}
		event.GenID(ctx)
		s.NoError(repo.Save(ctx, event))

		retrieved, err := repo.GetByEventID(ctx, roomID, event.GetID())
		s.NoError(err)
		s.NotNil(retrieved)
		s.Equal(senderID, retrieved.SenderID)
	})
}

func (s *EventRepositoryTestSuite) TestEventTypes() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()
		messageTypes := []chatv1.RoomEventType{
			chatv1.RoomEventType_TEXT,
			chatv1.RoomEventType_EVENT,
		}

		for _, msgType := range messageTypes {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: msgType.String(),
				Content:     data.JSONMap{"data": "test"},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		events, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.GreaterOrEqual(len(events), 2)
	})
}

func (s *EventRepositoryTestSuite) TestPagination() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomEventRepository(dbPool, workMan)

		roomID := util.IDString()

		for range 20 {
			event := &models.RoomEvent{
				RoomID:      roomID,
				SenderID:    util.IDString(),
				MessageType: chatv1.RoomEventType_TEXT.String(),
				Content:     data.JSONMap{"text": util.RandomString(10)},
			}
			event.GenID(ctx)
			s.NoError(repo.Save(ctx, event))
		}

		page1, err := repo.GetByRoomID(ctx, roomID, 10)
		s.NoError(err)
		s.Len(page1, 10)

		page2, err := repo.GetHistory(ctx, roomID, page1[len(page1)-1].GetID(), "", 10)
		s.NoError(err)
		s.GreaterOrEqual(len(page2), 1)
	})
}
