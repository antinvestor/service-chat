package repository

import (
	"context"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame"
)

func Migrate(ctx context.Context, svc *frame.Service, migrationPath string) error {
	return svc.MigrateDatastore(ctx, migrationPath,
		&models.Room{}, &models.RoomSubscription{}, &models.RoomEvent{}, &models.RoomOutbox{}, &models.RoomCall{})
}
