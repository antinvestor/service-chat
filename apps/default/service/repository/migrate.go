package repository

import (
	"context"
	"errors"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore"
)

func Migrate(ctx context.Context, dbManager datastore.Manager, migrationPath string) error {

	pool := dbManager.GetPool(ctx, datastore.DefaultMigrationPoolName)
	if pool == nil {
		return errors.New("datastore pool is not initialised")
	}

	return dbManager.Migrate(ctx, pool, migrationPath,
		&models.Room{}, &models.RoomSubscription{}, &models.RoomEvent{}, &models.RoomOutbox{}, &models.RoomCall{})
}
