package repository

import (
	"context"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/framedata"
)

type roomEventRepository struct {
	framedata.BaseRepository[*models.RoomEvent]
}

// GetByID retrieves a room event by its ID.
func (rer *roomEventRepository) GetByID(ctx context.Context, id string) (*models.RoomEvent, error) {
	event := &models.RoomEvent{}
	err := rer.Svc().DB(ctx, true).First(event, "id = ?", id).Error
	return event, err
}

// Save creates or updates a room event.
func (rer *roomEventRepository) Save(ctx context.Context, event *models.RoomEvent) error {
	return rer.Svc().DB(ctx, false).Save(event).Error
}

// Delete soft deletes a room event by its ID.
func (rer *roomEventRepository) Delete(ctx context.Context, id string) error {
	event, err := rer.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return rer.Svc().DB(ctx, false).Delete(event).Error
}

// GetByRoomID retrieves all events for a specific room, ordered by ID (naturally time-sorted).
func (rer *roomEventRepository) GetByRoomID(
	ctx context.Context,
	roomID string,
	limit int,
) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	query := rer.Svc().DB(ctx, true).
		Unscoped(). // Disable GORM's automatic soft delete filtering
		Where("room_id = ? AND deleted_at = 0", roomID).
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
	query := rer.Svc().DB(ctx, true).
		Unscoped(). // Disable GORM's automatic soft delete filtering
		Where("room_id = ? AND deleted_at = 0", roomID)

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
	err := rer.Svc().DB(ctx, true).
		Unscoped().
		Where("room_id = ? AND id = ? AND deleted_at = 0", roomID, eventID).
		First(event).Error
	return event, err
}

// CountByRoomID counts the total number of events in a room.
func (rer *roomEventRepository) CountByRoomID(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rer.Svc().DB(ctx, true).
		Model(&models.RoomEvent{}).
		Unscoped().
		Where("room_id = ? AND deleted_at = 0", roomID).
		Count(&count).Error
	return count, err
}

// NewRoomEventRepository creates a new room event repository instance.
func NewRoomEventRepository(service *frame.Service) RoomEventRepository {
	return &roomEventRepository{
		BaseRepository: framedata.NewBaseRepository[*models.RoomEvent](
			service,
			func() *models.RoomEvent { return &models.RoomEvent{} },
		),
	}
}
