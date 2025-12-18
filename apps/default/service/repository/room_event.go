package repository

import (
	"context"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

type roomEventRepository struct {
	datastore.BaseRepository[*models.RoomEvent]
}

// NewRoomEventRepository creates a new room event repository instance.
func NewRoomEventRepository(ctx context.Context, dbPool pool.Pool, workMan workerpool.Manager) RoomEventRepository {
	return &roomEventRepository{
		BaseRepository: datastore.NewBaseRepository[*models.RoomEvent](
			ctx, dbPool, workMan, func() *models.RoomEvent { return &models.RoomEvent{} },
		),
	}
}

// GetByRoomID retrieves all events for a specific room, ordered by ID (naturally time-sorted).
func (rer *roomEventRepository) GetByRoomID(
	ctx context.Context,
	roomID string,
	limit int,
) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	query := rer.Pool().DB(ctx, true).
		Unscoped(). // Disable GORM's automatic soft delete filtering
		Where("room_id = ? AND deleted_at IS NULL", roomID).
		Order("id ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	return events, err
}

// GetHistory retrieves room events with pagination support.
// beforeEventID: get events before this event ID (exclusive)
// afterEventID: get events after this event ID (exclusive)
// limit: maximum number of events to return
func (rer *roomEventRepository) GetHistory(
	ctx context.Context,
	roomID string,
	beforeEventID string,
	afterEventID string,
	limit int,
) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	query := rer.Pool().DB(ctx, true).
		Unscoped(). // Disable GORM's automatic soft delete filtering
		Where("room_id = ? AND deleted_at IS NULL", roomID)

	if beforeEventID != "" {
		query = query.Where("id < ?", beforeEventID)
	}

	if afterEventID != "" {
		query = query.Where("id > ?", afterEventID)
	}

	// Order by ID descending for "before" queries, ascending for "after"
	if beforeEventID != "" {
		query = query.Order("id DESC")
	} else {
		query = query.Order("id ASC")
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	return events, err
}

// GetByEventID retrieves a room event by its event ID (xid).
func (rer *roomEventRepository) GetByEventID(ctx context.Context, roomID, eventID string) (*models.RoomEvent, error) {
	event := &models.RoomEvent{}
	err := rer.Pool().DB(ctx, true).
		Unscoped().
		Where("room_id = ? AND id = ? AND deleted_at IS NULL", roomID, eventID).
		First(event).Error
	return event, err
}

// CountByRoomID counts the total number of events in a room.
func (rer *roomEventRepository) CountByRoomID(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rer.Pool().DB(ctx, true).
		Model(&models.RoomEvent{}).
		Unscoped().
		Where("room_id = ? AND deleted_at IS NULL", roomID).
		Count(&count).Error
	return count, err
}

// ExistsByIDs checks if any of the given event IDs already exist.
// Returns a map of eventID -> exists for efficient deduplication.
// This is used to implement idempotency - preventing duplicate message inserts.
func (rer *roomEventRepository) ExistsByIDs(ctx context.Context, eventIDs []string) (map[string]bool, error) {
	result := make(map[string]bool, len(eventIDs))

	// Initialize all as not existing
	for _, id := range eventIDs {
		result[id] = false
	}

	if len(eventIDs) == 0 {
		return result, nil
	}

	// Query for existing IDs
	var existingIDs []string
	err := rer.Pool().DB(ctx, true).
		Model(&models.RoomEvent{}).
		Unscoped().
		Where("id IN ? AND deleted_at IS NULL", eventIDs).
		Pluck("id", &existingIDs).Error
	if err != nil {
		return nil, err
	}

	// Mark existing IDs
	for _, id := range existingIDs {
		result[id] = true
	}

	return result, nil
}
