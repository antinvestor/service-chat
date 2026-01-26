package queues

import (
	"context"
	"testing"

	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

// mockPublisher implements queue.Publisher for testing.
type mockPublisher struct {
	published    []mockPublished
	publishError error
}

type mockPublished struct {
	payload any
	headers map[string]string
}

func (m *mockPublisher) Initiated() bool                       { return true }
func (m *mockPublisher) Ref() string                           { return "mock" }
func (m *mockPublisher) Init(_ context.Context) error          { return nil }
func (m *mockPublisher) Stop(_ context.Context) error          { return nil }
func (m *mockPublisher) As(_ any) bool                         { return false }
func (m *mockPublisher) Publish(_ context.Context, payload any, headers ...map[string]string) error {
	if m.publishError != nil {
		return m.publishError
	}
	var h map[string]string
	if len(headers) > 0 {
		h = headers[0]
	}
	m.published = append(m.published, mockPublished{payload: payload, headers: h})
	return nil
}

// mockQueueManager implements queue.Manager for testing.
type mockQueueManager struct {
	publishers      map[string]*mockPublisher
	getPublisherErr error
}

func newMockQueueManager() *mockQueueManager {
	return &mockQueueManager{
		publishers: make(map[string]*mockPublisher),
	}
}

func (m *mockQueueManager) AddPublisher(_ context.Context, _ string, _ string) error { return nil }
func (m *mockQueueManager) DiscardPublisher(_ context.Context, _ string) error       { return nil }
func (m *mockQueueManager) AddSubscriber(_ context.Context, _ string, _ string, _ ...queue.SubscribeWorker) error {
	return nil
}
func (m *mockQueueManager) DiscardSubscriber(_ context.Context, _ string) error { return nil }
func (m *mockQueueManager) GetSubscriber(_ string) (queue.Subscriber, error)    { return nil, nil }
func (m *mockQueueManager) Publish(_ context.Context, _ string, _ any, _ ...map[string]string) error {
	return nil
}
func (m *mockQueueManager) Init(_ context.Context) error { return nil }
func (m *mockQueueManager) GetPublisher(name string) (queue.Publisher, error) {
	if m.getPublisherErr != nil {
		return nil, m.getPublisherErr
	}
	pub, ok := m.publishers[name]
	if !ok {
		pub = &mockPublisher{}
		m.publishers[name] = pub
	}
	return pub, nil
}

func defaultTestConfig() *config.ChatConfig {
	return &config.ChatConfig{
		QueueDeadLetterName:           "dead.letter.queue",
		QueueDeadLetterURI:            "mem://dead.letter.queue",
		MaxDeliveryRetries:            5,
		QueueDeviceEventDeliveryName:  "device.event.delivery",
		QueueOfflineEventDeliveryName: "offline.event.delivery",
	}
}

func TestShouldDeadLetter_BelowMax(t *testing.T) {
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, newMockQueueManager())

	assert.False(t, dlp.ShouldDeadLetter(0))
	assert.False(t, dlp.ShouldDeadLetter(1))
	assert.False(t, dlp.ShouldDeadLetter(4))
}

func TestShouldDeadLetter_AtMax(t *testing.T) {
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, newMockQueueManager())

	// MaxDeliveryRetries=5 means retry counts 0-4 are OK, 5+ should dead-letter
	assert.True(t, dlp.ShouldDeadLetter(5))
}

func TestShouldDeadLetter_AboveMax(t *testing.T) {
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, newMockQueueManager())

	assert.True(t, dlp.ShouldDeadLetter(6))
	assert.True(t, dlp.ShouldDeadLetter(100))
}

func TestShouldDeadLetter_ZeroMaxRetries(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.MaxDeliveryRetries = 0
	dlp := NewDeadLetterPublisher(cfg, newMockQueueManager())

	// With max retries = 0, even the first attempt should dead-letter
	assert.True(t, dlp.ShouldDeadLetter(0))
}

func TestPublish_Success(t *testing.T) {
	qm := newMockQueueManager()
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}
	headers := map[string]string{"profile_id": "user123"}

	err := dlp.Publish(ctx, msg, "device.event.delivery", "test error", headers)
	require.NoError(t, err)

	// Verify the message was published to the DLQ
	pub := qm.publishers[cfg.QueueDeadLetterName]
	require.NotNil(t, pub)
	require.Len(t, pub.published, 1)

	// Verify headers contain original headers plus DLQ context
	pubHeaders := pub.published[0].headers
	assert.Equal(t, "user123", pubHeaders[internal.HeaderProfileID])
	assert.Equal(t, "device.event.delivery", pubHeaders[internal.HeaderDLQOriginalQueue])
	assert.Equal(t, "test error", pubHeaders[internal.HeaderDLQErrorMessage])
}

func TestPublish_PreservesOriginalHeaders(t *testing.T) {
	qm := newMockQueueManager()
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}
	originalHeaders := map[string]string{
		internal.HeaderProfileID: "user456",
		internal.HeaderDeviceID:  "device789",
		internal.HeaderShardID:   "2",
		internal.HeaderPriority:  "high",
	}

	err := dlp.Publish(ctx, msg, "test.queue", "delivery failed", originalHeaders)
	require.NoError(t, err)

	pub := qm.publishers[cfg.QueueDeadLetterName]
	pubHeaders := pub.published[0].headers

	// All original headers should be preserved
	assert.Equal(t, "user456", pubHeaders[internal.HeaderProfileID])
	assert.Equal(t, "device789", pubHeaders[internal.HeaderDeviceID])
	assert.Equal(t, "2", pubHeaders[internal.HeaderShardID])
	assert.Equal(t, "high", pubHeaders[internal.HeaderPriority])

	// DLQ headers should be added
	assert.Equal(t, "test.queue", pubHeaders[internal.HeaderDLQOriginalQueue])
	assert.Equal(t, "delivery failed", pubHeaders[internal.HeaderDLQErrorMessage])
}

func TestPublish_NilHeaders(t *testing.T) {
	qm := newMockQueueManager()
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}

	err := dlp.Publish(ctx, msg, "test.queue", "error msg", nil)
	require.NoError(t, err)

	pub := qm.publishers[cfg.QueueDeadLetterName]
	pubHeaders := pub.published[0].headers
	assert.Equal(t, "test.queue", pubHeaders[internal.HeaderDLQOriginalQueue])
	assert.Equal(t, "error msg", pubHeaders[internal.HeaderDLQErrorMessage])
}

func TestPublish_GetPublisherError(t *testing.T) {
	qm := newMockQueueManager()
	qm.getPublisherErr = assert.AnError
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}

	err := dlp.Publish(ctx, msg, "test.queue", "error msg", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get dead-letter publisher")
}

func TestPublish_PublishError(t *testing.T) {
	qm := newMockQueueManager()
	pub := &mockPublisher{publishError: assert.AnError}
	qm.publishers["dead.letter.queue"] = pub

	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}

	err := dlp.Publish(ctx, msg, "test.queue", "error msg", nil)
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestPublish_DoesNotMutateOriginalHeaders(t *testing.T) {
	qm := newMockQueueManager()
	cfg := defaultTestConfig()
	dlp := NewDeadLetterPublisher(cfg, qm)

	ctx := context.Background()
	msg := &emptypb.Empty{}
	originalHeaders := map[string]string{
		"key": "value",
	}

	err := dlp.Publish(ctx, msg, "test.queue", "error msg", originalHeaders)
	require.NoError(t, err)

	// Original headers should not be mutated
	assert.Len(t, originalHeaders, 1)
	assert.Equal(t, "value", originalHeaders["key"])
	_, hasDLQKey := originalHeaders[internal.HeaderDLQOriginalQueue]
	assert.False(t, hasDLQKey, "original headers should not contain DLQ keys")
}

func TestDeadLetterPublisher_IntegrationWithRetryCount(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.MaxDeliveryRetries = 3

	dlp := NewDeadLetterPublisher(cfg, newMockQueueManager())

	// Simulate retry progression
	for i := range int32(3) {
		assert.False(t, dlp.ShouldDeadLetter(i), "retry count %d should not dead-letter", i)
	}
	assert.True(t, dlp.ShouldDeadLetter(3), "retry count 3 should dead-letter")
}
