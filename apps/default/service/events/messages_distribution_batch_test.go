package events_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	commonv1 "buf.build/gen/go/antinvestor/common/protocolbuffers/go/common/v1"
	"github.com/antinvestor/service-chat/apps/default/service/events"
	"github.com/antinvestor/service-chat/apps/default/service/models"
	"github.com/antinvestor/service-chat/apps/default/service/repository"
	frevents "github.com/pitabwire/frame/events"
	"github.com/pitabwire/frame/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock Repo
type mockSubscriptionRepo struct {
	repository.RoomSubscriptionRepository
	getByRoomIDPagedFunc func(ctx context.Context, roomID string, lastID string, limit int) ([]*models.RoomSubscription, error)
}

func (m *mockSubscriptionRepo) GetByRoomIDPaged(ctx context.Context, roomID string, lastID string, limit int) ([]*models.RoomSubscription, error) {
	if m.getByRoomIDPagedFunc != nil {
		return m.getByRoomIDPagedFunc(ctx, roomID, lastID, limit)
	}
	return nil, errors.New("not implemented")
}

// Mock Events Manager
type mockEventsManager struct {
	mock.Mock
}

func (m *mockEventsManager) Emit(ctx context.Context, eventName string, payload interface{}) error {
	args := m.Called(ctx, eventName, payload)
	return args.Error(0)
}

func (m *mockEventsManager) Add(e frevents.EventI) {}
func (m *mockEventsManager) Get(name string) (frevents.EventI, error) {
	return nil, nil
}
func (m *mockEventsManager) Handler() queue.SubscribeWorker {
	return nil
}

func TestRoomOutboxLoggingQueue_Execute_RecursiveBatching(t *testing.T) {
	// Setup
	mockRepo := &mockSubscriptionRepo{}
	mockEvents := new(mockEventsManager)

	// SUT
	queue := events.NewRoomOutboxLoggingQueue(context.Background(), mockRepo, mockEvents)

	// Test Data
	roomID := "room-123"
	evtLink := &eventsv1.Link{
		RoomId: roomID,
		Source: &commonv1.ContactLink{ProfileId: "sender-1"},
	}
	payload := &events.RoomOutboxPayload{
		Link:   evtLink,
		Cursor: "",
	}

	callCount := 0
	mockRepo.getByRoomIDPagedFunc = func(ctx context.Context, rid string, lastID string, limit int) ([]*models.RoomSubscription, error) {
		callCount++
		if rid != roomID {
			return nil, nil
		}

		if lastID == "" {
			// First batch: 1000 items
			subs := make([]*models.RoomSubscription, 1000)
			for i := 0; i < 1000; i++ {
				subs[i] = &models.RoomSubscription{
					RoomID:    roomID,
					ProfileID: fmt.Sprintf("sub-%d", i),
				}
				subs[i].ID = strconv.Itoa(i + 1)
			}
			return subs, nil
		}

		// End (simulating only 1000 subscribers, so no more batches)
		return []*models.RoomSubscription{}, nil
	}

	// Expectations - First batch: 1000 destinations
	mockEvents.On("Emit", mock.Anything, events.RoomFanoutEventName, mock.MatchedBy(func(b *eventsv1.Broadcast) bool {
		return len(b.Destinations) == 1000
	})).Return(nil).Once()

	// Expect a recursive emit for the next batch
	mockEvents.On("Emit", mock.Anything, events.RoomOutboxLoggingEventName, mock.MatchedBy(func(p *events.RoomOutboxPayload) bool {
		return p.Cursor == "1000" && p.Link == evtLink
	})).Return(nil).Once()

	// Execute
	err := queue.Execute(context.Background(), payload)

	// Verify
	assert.NoError(t, err)
	mockEvents.AssertExpectations(t)
}

func TestRoomOutboxLoggingQueue_Execute_LastBatch(t *testing.T) {
	// Setup
	mockRepo := &mockSubscriptionRepo{}
	mockEvents := new(mockEventsManager)

	// SUT
	queue := events.NewRoomOutboxLoggingQueue(context.Background(), mockRepo, mockEvents)

	// Test Data - simulating a continuation batch
	roomID := "room-123"
	evtLink := &eventsv1.Link{
		RoomId: roomID,
		Source: &commonv1.ContactLink{ProfileId: "sender-1"},
	}
	payload := &events.RoomOutboxPayload{
		Link:   evtLink,
		Cursor: "1000", // Continuing from previous batch
	}

	mockRepo.getByRoomIDPagedFunc = func(ctx context.Context, rid string, lastID string, limit int) ([]*models.RoomSubscription, error) {
		if lastID == "1000" {
			// Return fewer than batch size (last batch)
			subs := make([]*models.RoomSubscription, 500)
			for i := 0; i < 500; i++ {
				subs[i] = &models.RoomSubscription{
					RoomID:    roomID,
					ProfileID: fmt.Sprintf("sub-%d", 1000+i),
				}
				subs[i].ID = strconv.Itoa(1001 + i)
			}
			return subs, nil
		}
		return []*models.RoomSubscription{}, nil
	}

	// Expect broadcast for final batch
	mockEvents.On("Emit", mock.Anything, events.RoomFanoutEventName, mock.MatchedBy(func(b *eventsv1.Broadcast) bool {
		return len(b.Destinations) == 500
	})).Return(nil).Once()

	// Should NOT emit a recursive job (fewer than 1000 subscribers returned)

	// Execute
	err := queue.Execute(context.Background(), payload)

	// Verify
	assert.NoError(t, err)
	mockEvents.AssertExpectations(t)
}
