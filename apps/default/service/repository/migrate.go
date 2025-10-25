package repository

import (
	"context"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore/pool"
)

func Migrate(ctx context.Context, dbPool pool.Pool, migrationPath string) error {
	return dbPool.Migrate(ctx, migrationPath,
		&models.Room{}, &models.RoomSubscription{}, &models.RoomEvent{}, &models.RoomOutbox{}, &models.RoomCall{})
}
