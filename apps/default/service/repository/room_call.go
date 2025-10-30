package repository

import (
	"context"
	"time"

	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/pitabwire/frame/datastore"
	"github.com/pitabwire/frame/datastore/pool"
	"github.com/pitabwire/frame/workerpool"
)

const (
	CallStatusRinging = "ringing"
	CallStatusActive  = "active"
	CallStatusEnded   = "ended"
)

type roomCallRepository struct {
	datastore.BaseRepository[*models.RoomCall]
}

// GetByCallID retrieves a room call by its call ID.
func (rcr *roomCallRepository) GetByCallID(ctx context.Context, callID string) (*models.RoomCall, error) {
	call := &models.RoomCall{}
	err := rcr.Pool().DB(ctx, true).
		Where("call_id = ?", callID).
		First(call).Error
	return call, err
}

// GetByRoomID retrieves all calls for a specific room.
func (rcr *roomCallRepository) GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomCall, error) {
	var calls []*models.RoomCall
	query := rcr.Pool().DB(ctx, true).
		Where("room_id = ?", roomID).
		Order("started_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&calls).Error
	return calls, err
}

// GetActiveCallByRoomID retrieves the active call for a room, if any.
func (rcr *roomCallRepository) GetActiveCallByRoomID(ctx context.Context, roomID string) (*models.RoomCall, error) {
	call := &models.RoomCall{}
	err := rcr.Pool().DB(ctx, true).
		Where("room_id = ? AND status IN ?", roomID, []string{CallStatusRinging, CallStatusActive}).
		Order("started_at DESC").
		First(call).Error
	return call, err
}

// GetByStatus retrieves calls by status.
func (rcr *roomCallRepository) GetByStatus(ctx context.Context, status string, limit int) ([]*models.RoomCall, error) {
	var calls []*models.RoomCall
	query := rcr.Pool().DB(ctx, true).
		Where("status = ?", status).
		Order("started_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&calls).Error
	return calls, err
}

// UpdateStatus updates the status of a call.
func (rcr *roomCallRepository) UpdateStatus(ctx context.Context, id, status string) error {
	updates := map[string]interface{}{
		"status": status,
	}

	// If ending the call, set the ended_at timestamp
	if status == CallStatusEnded {
		updates["ended_at"] = time.Now()
	}

	return rcr.Pool().DB(ctx, false).
		Model(&models.RoomCall{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateSFUNode updates the SFU node ID for a call.
func (rcr *roomCallRepository) UpdateSFUNode(ctx context.Context, id, sfuNodeID string) error {
	return rcr.Pool().DB(ctx, false).
		Model(&models.RoomCall{}).
		Where("id = ?", id).
		Update("sfu_node_id", sfuNodeID).Error
}

// EndCall marks a call as ended and sets the ended_at timestamp.
func (rcr *roomCallRepository) EndCall(ctx context.Context, id string) error {
	return rcr.UpdateStatus(ctx, id, CallStatusEnded)
}

// GetTimedOutCalls retrieves calls that have been ringing for longer than the timeout duration.
func (rcr *roomCallRepository) GetTimedOutCalls(
	ctx context.Context,
	timeout time.Duration,
) ([]*models.RoomCall, error) {
	cutoffTime := time.Now().Add(-timeout)
	var calls []*models.RoomCall
	err := rcr.Pool().DB(ctx, true).
		Where("status = ? AND started_at < ?", CallStatusRinging, cutoffTime).
		Find(&calls).Error
	return calls, err
}

// GetCallDuration calculates the duration of a call.
func (rcr *roomCallRepository) GetCallDuration(ctx context.Context, id string) (time.Duration, error) {
	call, err := rcr.GetByID(ctx, id)
	if err != nil {
		return 0, err
	}

	endTime := call.EndedAt
	if call.Status != CallStatusEnded {
		endTime = time.Now()
	}

	duration := endTime.Sub(call.StartedAt)
	return duration, nil
}

// CountActiveCallsByRoomID counts the number of active calls in a room.
func (rcr *roomCallRepository) CountActiveCallsByRoomID(ctx context.Context, roomID string) (int64, error) {
	var count int64
	err := rcr.Pool().DB(ctx, true).
		Model(&models.RoomCall{}).
		Where("room_id = ? AND status IN ?", roomID, []string{CallStatusRinging, CallStatusActive}).
		Count(&count).Error
	return count, err
}

// GetCallsBySFUNode retrieves all calls assigned to a specific SFU node.
func (rcr *roomCallRepository) GetCallsBySFUNode(ctx context.Context, sfuNodeID string) ([]*models.RoomCall, error) {
	var calls []*models.RoomCall
	err := rcr.Pool().DB(ctx, true).
		Where("sfu_node_id = ? AND status IN ?", sfuNodeID, []string{CallStatusRinging, CallStatusActive}).
		Find(&calls).Error
	return calls, err
}

// NewRoomCallRepository creates a new room call repository instance.
func NewRoomCallRepository(ctx context.Context, dbPool pool.Pool, workMan workerpool.Manager) RoomCallRepository {
	return &roomCallRepository{
		BaseRepository: datastore.NewBaseRepository[*models.RoomCall](
			ctx, dbPool, workMan, func() *models.RoomCall { return &models.RoomCall{} },
		),
	}
}
