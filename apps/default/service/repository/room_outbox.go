package repository

import (
	"context"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/framedata"
	"gorm.io/gorm"
)

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusSent       = "sent"
	OutboxStatusFailed     = "failed"
)

type roomOutboxRepository struct {
	framedata.BaseRepository[*models.RoomOutbox]
}

// GetByID retrieves an outbox entry by its ID.
func (ror *roomOutboxRepository) GetByID(ctx context.Context, id string) (*models.RoomOutbox, error) {
	outbox := &models.RoomOutbox{}
	err := ror.Svc().DB(ctx, true).First(outbox, "id = ?", id).Error
	return outbox, err
}

// Save creates or updates an outbox entry.
func (ror *roomOutboxRepository) Save(ctx context.Context, outbox *models.RoomOutbox) error {
	return ror.Svc().DB(ctx, false).Save(outbox).Error
}

// Delete soft deletes an outbox entry by its ID.
func (ror *roomOutboxRepository) Delete(ctx context.Context, id string) error {
	outbox, err := ror.GetByID(ctx, id)
	if err != nil {
		return err
	}
	return ror.Svc().DB(ctx, false).Delete(outbox).Error
}

// GetByEventID retrieves an outbox entry by event ID.
func (ror *roomOutboxRepository) GetByEventID(ctx context.Context, eventID string) (*models.RoomOutbox, error) {
	outbox := &models.RoomOutbox{}
	err := ror.Svc().DB(ctx, true).
		Where("event_id = ?", eventID).
		First(outbox).Error
	return outbox, err
}

// GetPendingEntries retrieves outbox entries with pending status, ordered by creation time.
func (ror *roomOutboxRepository) GetPendingEntries(ctx context.Context, limit int) ([]*models.RoomOutbox, error) {
	var entries []*models.RoomOutbox
	query := ror.Svc().DB(ctx, true).
		Where("status = ?", OutboxStatusPending).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// GetFailedEntries retrieves outbox entries with failed status that can be retried.
func (ror *roomOutboxRepository) GetFailedEntries(
	ctx context.Context,
	maxRetries int,
	limit int,
) ([]*models.RoomOutbox, error) {
	var entries []*models.RoomOutbox
	query := ror.Svc().DB(ctx, true).
		Where("status = ? AND retry_count < ?", OutboxStatusFailed, maxRetries).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// UpdateStatus updates the status of an outbox entry.
func (ror *roomOutboxRepository) UpdateStatus(ctx context.Context, id, status string) error {
	return ror.Svc().DB(ctx, false).
		Model(&models.RoomOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status": status,
		}).Error
}

// UpdateStatusWithError updates the status and error message of an outbox entry.
func (ror *roomOutboxRepository) UpdateStatusWithError(ctx context.Context, id, status, errorMsg string) error {
	return ror.Svc().DB(ctx, false).
		Model(&models.RoomOutbox{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        status,
			"error_message": errorMsg,
		}).Error
}

// IncrementRetryCount increments the retry count for an outbox entry.
func (ror *roomOutboxRepository) IncrementRetryCount(ctx context.Context, id string) error {
	return ror.Svc().DB(ctx, false).
		Model(&models.RoomOutbox{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
}

// GetByRoomID retrieves all outbox entries for a specific room.
func (ror *roomOutboxRepository) GetByRoomID(
	ctx context.Context,
	roomID string,
	limit int,
) ([]*models.RoomOutbox, error) {
	var entries []*models.RoomOutbox
	query := ror.Svc().DB(ctx, true).
		Where("room_id = ?", roomID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// GetBacklogCount counts the number of pending and processing outbox entries.
func (ror *roomOutboxRepository) GetBacklogCount(ctx context.Context) (int64, error) {
	var count int64
	err := ror.Svc().DB(ctx, true).
		Model(&models.RoomOutbox{}).
		Where("status IN ?", []string{OutboxStatusPending, OutboxStatusProcessing}).
		Count(&count).Error
	return count, err
}

// CleanupOldEntries deletes successfully sent entries older than the specified duration.
func (ror *roomOutboxRepository) CleanupOldEntries(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)
	result := ror.Svc().DB(ctx, false).
		Where("status = ? AND created_at < ?", OutboxStatusSent, cutoffTime).
		Delete(&models.RoomOutbox{})
	return result.RowsAffected, result.Error
}

// GetByStatus retrieves outbox entries by status.
func (ror *roomOutboxRepository) GetByStatus(
	ctx context.Context,
	status string,
	limit int,
) ([]*models.RoomOutbox, error) {
	var entries []*models.RoomOutbox
	query := ror.Svc().DB(ctx, true).
		Where("status = ?", status).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// GetPendingBySubscription retrieves pending outbox entries for a specific subscription.
func (ror *roomOutboxRepository) GetPendingBySubscription(
	ctx context.Context,
	subscriptionID string,
	limit int,
) ([]*models.RoomOutbox, error) {
	var entries []*models.RoomOutbox
	query := ror.Svc().DB(ctx, true).
		Where("subscription_id = ? AND status = ?", subscriptionID, OutboxStatusPending).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&entries).Error
	return entries, err
}

// NewRoomOutboxRepository creates a new room outbox repository instance.
func NewRoomOutboxRepository(service *frame.Service) RoomOutboxRepository {
	return &roomOutboxRepository{
		BaseRepository: framedata.NewBaseRepository[*models.RoomOutbox](
			service,
			func() *models.RoomOutbox { return &models.RoomOutbox{} },
		),
	}
}
