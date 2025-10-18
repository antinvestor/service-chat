package repository

import (
	"context"
	"fmt"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/framedata"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/antinvestor/service-chat/apps/default/service/models"
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

// GetByRoomID retrieves all events for a specific room, ordered by sequence.
func (rer *roomEventRepository) GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	query := rer.Svc().DB(ctx, true).
		Where("room_id = ?", roomID).
		Order("sequence ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	return events, err
}

// GetHistory retrieves room events with pagination support.
// beforeSequence: get events before this sequence (exclusive)
// afterSequence: get events after this sequence (exclusive)
// limit: maximum number of events to return
func (rer *roomEventRepository) GetHistory(
	ctx context.Context,
	roomID string,
	beforeSequence int64,
	afterSequence int64,
	limit int,
) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	query := rer.Svc().DB(ctx, true).Where("room_id = ?", roomID)

	if beforeSequence > 0 {
		query = query.Where("sequence < ?", beforeSequence)
	}

	if afterSequence > 0 {
		query = query.Where("sequence > ?", afterSequence)
	}

	// Order by sequence descending for "before" queries, ascending for "after"
	if beforeSequence > 0 {
		query = query.Order("sequence DESC")
	} else {
		query = query.Order("sequence ASC")
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&events).Error
	return events, err
}

// GetBySequenceRange retrieves events within a sequence range (inclusive).
func (rer *roomEventRepository) GetBySequenceRange(
	ctx context.Context,
	roomID string,
	fromSequence int64,
	toSequence int64,
) ([]*models.RoomEvent, error) {
	var events []*models.RoomEvent
	err := rer.Svc().DB(ctx, true).
		Where("room_id = ? AND sequence >= ? AND sequence <= ?", roomID, fromSequence, toSequence).
		Order("sequence ASC").
		Find(&events).Error
	return events, err
}

// GetNextSequence generates the next monotonic sequence number for a room.
// Uses row-level locking to ensure gapless sequences.
func (rer *roomEventRepository) GetNextSequence(ctx context.Context, roomID string) (int64, error) {
	var maxSequence int64

	err := rer.Svc().DB(ctx, false).Transaction(func(tx *gorm.DB) error {
		// Lock the room row to prevent concurrent sequence generation
		var room models.Room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", roomID).
			First(&room).Error; err != nil {
			return fmt.Errorf("failed to lock room: %w", err)
		}

		// Get the maximum sequence for this room
		result := tx.Model(&models.RoomEvent{}).
			Where("room_id = ?", roomID).
			Select("COALESCE(MAX(sequence), 0)").
			Scan(&maxSequence)

		if result.Error != nil {
			return fmt.Errorf("failed to get max sequence: %w", result.Error)
		}

		maxSequence++
		return nil
	})

	return maxSequence, err
}

// GetLatestSequence retrieves the latest sequence number for a room.
func (rer *roomEventRepository) GetLatestSequence(ctx context.Context, roomID string) (int64, error) {
	var maxSequence int64
	err := rer.Svc().DB(ctx, true).
		Model(&models.RoomEvent{}).
		Where("room_id = ?", roomID).
		Select("COALESCE(MAX(sequence), 0)").
		Scan(&maxSequence).Error
	return maxSequence, err
}

// GetByEventID retrieves a room event by its event ID (xid).
func (rer *roomEventRepository) GetByEventID(ctx context.Context, roomID, eventID string) (*models.RoomEvent, error) {
	event := &models.RoomEvent{}
	err := rer.Svc().DB(ctx, true).
		Where("room_id = ? AND id = ?", roomID, eventID).
		First(event).Error
	return event, err
}

// CountByRoomID counts the total number of events in a room.
func (rer *roomEventRepository) CountByRoomID(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rer.Svc().DB(ctx, true).
		Model(&models.RoomEvent{}).
		Where("room_id = ?", roomID).
		Count(&count).Error
	return count, err
}

// NewRoomEventRepository creates a new room event repository instance.
func NewRoomEventRepository(service *frame.Service) RoomEventRepository {
	return &roomEventRepository{
		BaseRepository: framedata.NewBaseRepository[*models.RoomEvent](service, func() *models.RoomEvent { return &models.RoomEvent{} }),
	}
}
