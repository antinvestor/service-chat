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

type RoomRepositoryTestSuite struct {
	tests.BaseTestSuite
}

func TestRoomRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RoomRepositoryTestSuite))
}

func (s *RoomRepositoryTestSuite) TestCreateRoom() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomRepository(dbPool, workMan)

		room := &models.Room{
			RoomType:    "group",
			Name:        "Test Room",
			Description: "Test Description",
			IsPublic:    true,
			Properties:  data.JSONMap{"key": "value"},
		}
		room.GenID(ctx)

		err := repo.Save(ctx, room)
		s.NoError(err)
		s.NotEmpty(room.GetID())

		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.NoError(err)
		s.Equal(room.Name, retrieved.Name)
		s.Equal(room.Description, retrieved.Description)
		s.Equal(room.IsPublic, retrieved.IsPublic)
	})
}

func (s *RoomRepositoryTestSuite) TestUpdateRoom() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomRepository(dbPool, workMan)

		room := &models.Room{
			RoomType:    "group",
			Name:        "Original Name",
			Description: "Original Description",
			IsPublic:    true,
		}
		room.GenID(ctx)
		err := repo.Save(ctx, room)
		s.NoError(err)

		room.Name = "Updated Name"
		room.Description = "Updated Description"
		err = repo.Save(ctx, room)
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.NoError(err)
		s.Equal("Updated Name", retrieved.Name)
		s.Equal("Updated Description", retrieved.Description)
	})
}

func (s *RoomRepositoryTestSuite) TestDeleteRoom() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomRepository(dbPool, workMan)

		room := &models.Room{
			RoomType: "group",
			Name:     "Room to Delete",
			IsPublic: true,
		}
		room.GenID(ctx)
		err := repo.Save(ctx, room)
		s.NoError(err)

		err = repo.Delete(ctx, room.GetID())
		s.NoError(err)

		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.Error(err)
		s.Nil(retrieved)
	})
}

func (s *RoomRepositoryTestSuite) TestSearchRooms() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomRepository(dbPool, workMan)

		for range 3 {
			room := &models.Room{
				RoomType: "group",
				Name:     util.RandomString(10),
				IsPublic: true,
			}
			room.GenID(ctx)
			err := repo.Save(ctx, room)
			s.NoError(err)
		}
	})
}

func (s *RoomRepositoryTestSuite) TestGetRoomsByIDs() {
	frametests.WithTestDependancies(s.T(), nil, func(t *testing.T, dep *definition.DependancyOption) {
		ctx, svc := s.CreateService(t, dep)
		workMan, dbPool := s.GetRepoDeps(ctx, svc)
		repo := repository.NewRoomRepository(dbPool, workMan)

		room1 := &models.Room{RoomType: "group", Name: "Room 1", IsPublic: true}
		room1.GenID(ctx)
		room2 := &models.Room{RoomType: "group", Name: "Room 2", IsPublic: true}
		room2.GenID(ctx)

		s.NoError(repo.Save(ctx, room1))
		s.NoError(repo.Save(ctx, room2))

		retrieved1, err := repo.GetByID(ctx, room1.GetID())
		s.NoError(err)
		s.Equal(room1.Name, retrieved1.Name)

		retrieved2, err := repo.GetByID(ctx, room2.GetID())
		s.NoError(err)
		s.Equal(room2.Name, retrieved2.Name)
	})
}
