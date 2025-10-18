package repository

import (
	"context"
	"time"

	"github.com/pitabwire/frame/framedata"

	"github.com/antinvestor/service-chat/apps/default/service/models"
)

// RoomRepository defines the interface for room data access operations.
type RoomRepository interface {
	framedata.BaseRepository[*models.Room]
	GetByTenantAndType(ctx context.Context, tenantID, roomType string) ([]*models.Room, error)
	GetRoomsByProfileID(ctx context.Context, profileID string) ([]*models.Room, error)
}

// RoomEventRepository defines the interface for room event data access operations.
type RoomEventRepository interface {
	framedata.BaseRepository[*models.RoomEvent]
	GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomEvent, error)
	GetHistory(ctx context.Context, roomID string, beforeEventID, afterEventID string, limit int) ([]*models.RoomEvent, error)
	GetByEventID(ctx context.Context, roomID, eventID string) (*models.RoomEvent, error)
	CountByRoomID(ctx context.Context, roomID string) (int64, error)
}

// RoomOutboxRepository defines the interface for room outbox data access operations.
type RoomOutboxRepository interface {
	framedata.BaseRepository[*models.RoomOutbox]
	GetByEventID(ctx context.Context, eventID string) (*models.RoomOutbox, error)
	GetPendingEntries(ctx context.Context, limit int) ([]*models.RoomOutbox, error)
	GetFailedEntries(ctx context.Context, maxRetries, limit int) ([]*models.RoomOutbox, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateStatusWithError(ctx context.Context, id, status, errorMsg string) error
	IncrementRetryCount(ctx context.Context, id string) error
	GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomOutbox, error)
	GetBacklogCount(ctx context.Context) (int64, error)
	CleanupOldEntries(ctx context.Context, olderThan time.Duration) (int64, error)
	GetByStatus(ctx context.Context, status string, limit int) ([]*models.RoomOutbox, error)
	GetPendingBySubscription(ctx context.Context, subscriptionID string, limit int) ([]*models.RoomOutbox, error)
}

// RoomSubscriptionRepository defines the interface for room subscription data access operations.
type RoomSubscriptionRepository interface {
	framedata.BaseRepository[*models.RoomSubscription]
	GetByRoomAndProfile(ctx context.Context, roomID, profileID string) (*models.RoomSubscription, error)
	GetActiveByRoomAndProfile(ctx context.Context, roomID, profileID string) (*models.RoomSubscription, error)
	GetByRoomID(ctx context.Context, roomID string, activeOnly bool) ([]*models.RoomSubscription, error)
	GetByProfileID(ctx context.Context, profileID string, activeOnly bool) ([]*models.RoomSubscription, error)
	GetMembersByRoomID(ctx context.Context, roomID string) ([]string, error)
	GetByRole(ctx context.Context, roomID, role string) ([]*models.RoomSubscription, error)
	UpdateRole(ctx context.Context, id, role string) error
	UpdateLastReadEventID(ctx context.Context, id string, eventID string) error
	Deactivate(ctx context.Context, id string) error
	Activate(ctx context.Context, id string) error
	CountActiveMembers(ctx context.Context, roomID string) (int64, error)
	HasPermission(ctx context.Context, roomID, profileID, minRole string) (bool, error)
	IsActiveMember(ctx context.Context, roomID, profileID string) (bool, error)
	BulkCreate(ctx context.Context, subscriptions []*models.RoomSubscription) error
}

// RoomCallRepository defines the interface for room call data access operations.
type RoomCallRepository interface {
	framedata.BaseRepository[*models.RoomCall]
	GetByCallID(ctx context.Context, callID string) (*models.RoomCall, error)
	GetByRoomID(ctx context.Context, roomID string, limit int) ([]*models.RoomCall, error)
	GetActiveCallByRoomID(ctx context.Context, roomID string) (*models.RoomCall, error)
	GetByStatus(ctx context.Context, status string, limit int) ([]*models.RoomCall, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateSFUNode(ctx context.Context, id, sfuNodeID string) error
	EndCall(ctx context.Context, id string) error
	GetTimedOutCalls(ctx context.Context, timeout time.Duration) ([]*models.RoomCall, error)
	GetCallDuration(ctx context.Context, id string) (time.Duration, error)
	CountActiveCallsByRoomID(ctx context.Context, roomID string) (int64, error)
	GetCallsBySFUNode(ctx context.Context, sfuNodeID string) ([]*models.RoomCall, error)
}
