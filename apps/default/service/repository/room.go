package repository

import (
	"context"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

type roomRepository struct {
	datastore.BaseRepository[*models.Room]
}

// GetByTenantAndType retrieves rooms by tenant ID and room type.
func (rr *roomRepository) GetByTenantAndType(ctx context.Context, tenantID, roomType string) ([]*models.Room, error) {
	var rooms []*models.Room
	err := rr.Pool().DB(ctx, true).
		Where("tenant_id = ? AND room_type = ?", tenantID, roomType).
		Find(&rooms).Error
	return rooms, err
}

// GetRoomsByProfileID retrieves all rooms a profile is subscribed to.
func (rr *roomRepository) GetRoomsByProfileID(ctx context.Context, profileID string) ([]*models.Room, error) {
	var rooms []*models.Room
	err := rr.Pool().DB(ctx, true).
		Joins("JOIN room_subscriptions ON room_subscriptions.room_id = rooms.id").
		Where("room_subscriptions.profile_id = ? AND room_subscriptions.is_active = ?", profileID, true).
		Find(&rooms).Error
	return rooms, err
}

// NewRoomRepository creates a new room repository instance.
func NewRoomRepository(ctx context.Context, dbPool pool.Pool, workMan workerpool.Manager) RoomRepository {
	return &roomRepository{
		BaseRepository: datastore.NewBaseRepository[*models.Room](
			ctx, dbPool, workMan, func() *models.Room { return &models.Room{} },
		),
	}
}
