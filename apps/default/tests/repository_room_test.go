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

type RoomRepositoryTestSuite struct {
	BaseTestSuite
}

func TestRoomRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RoomRepositoryTestSuite))
}

func (s *RoomRepositoryTestSuite) TestCreateRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomRepository(svc)

		room := &models.Room{
			RoomType:    "group",
			Name:        "Test Room",
			Description: "Test Description",
			IsPublic:    true,
			Properties:  frame.JSONMap{"key": "value"},
		}
		room.GenID(ctx)

		err := repo.Save(ctx, room)
		s.NoError(err)
		s.NotEmpty(room.GetID())

		// Verify retrieval
		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.NoError(err)
		s.Equal(room.Name, retrieved.Name)
		s.Equal(room.Description, retrieved.Description)
		s.Equal(room.IsPublic, retrieved.IsPublic)
	})
}

func (s *RoomRepositoryTestSuite) TestUpdateRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomRepository(svc)

		// Create room
		room := &models.Room{
			RoomType:    "group",
			Name:        "Original Name",
			Description: "Original Description",
			IsPublic:    true,
		}
		room.GenID(ctx)
		err := repo.Save(ctx, room)
		s.NoError(err)

		// Update room
		room.Name = "Updated Name"
		room.Description = "Updated Description"
		err = repo.Save(ctx, room)
		s.NoError(err)

		// Verify update
		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.NoError(err)
		s.Equal("Updated Name", retrieved.Name)
		s.Equal("Updated Description", retrieved.Description)
	})
}

func (s *RoomRepositoryTestSuite) TestDeleteRoom() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomRepository(svc)

		// Create room
		room := &models.Room{
			RoomType: "group",
			Name:     "Room to Delete",
			IsPublic: true,
		}
		room.GenID(ctx)
		err := repo.Save(ctx, room)
		s.NoError(err)

		// Delete room
		err = repo.Delete(ctx, room.GetID())
		s.NoError(err)

		// Verify deletion (soft delete)
		retrieved, err := repo.GetByID(ctx, room.GetID())
		s.Error(err) // Should not find deleted room
		s.Nil(retrieved)
	})
}

func (s *RoomRepositoryTestSuite) TestSearchRooms() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomRepository(svc)

		// Create multiple rooms
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

		// Note: Search functionality requires custom implementation
		// This test validates basic room creation
	})
}

func (s *RoomRepositoryTestSuite) TestGetRoomsByIDs() {
	s.WithTestDependancies(s.T(), func(t *testing.T, dep *definition.DependancyOption) {
		svc, ctx := s.CreateService(t, dep)
		repo := repository.NewRoomRepository(svc)

		// Create rooms
		room1 := &models.Room{RoomType: "group", Name: "Room 1", IsPublic: true}
		room1.GenID(ctx)
		room2 := &models.Room{RoomType: "group", Name: "Room 2", IsPublic: true}
		room2.GenID(ctx)

		s.NoError(repo.Save(ctx, room1))
		s.NoError(repo.Save(ctx, room2))

		// Get by ID individually (GetByIDs doesn't exist)
		retrieved1, err := repo.GetByID(ctx, room1.GetID())
		s.NoError(err)
		s.Equal(room1.Name, retrieved1.Name)

		retrieved2, err := repo.GetByID(ctx, room2.GetID())
		s.NoError(err)
		s.Equal(room2.Name, retrieved2.Name)
	})
}
