package business //nolint:testpackage // Tests need access to unexported rate limiter and connection internals

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Token Bucket Tests ---

func TestTokenBucket_InitialBurst(t *testing.T) {
	tb := newTokenBucket(100, 20)

	// Should allow up to burst capacity immediately
	for i := range 20 {
		assert.True(t, tb.Allow(), "request %d should be allowed within burst", i)
	}

	// Next request should be denied (tokens exhausted)
	assert.False(t, tb.Allow(), "should deny when tokens exhausted")
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := newTokenBucket(100, 5) // 100 tokens/sec, burst of 5

	// Exhaust all tokens
	for range 5 {
		tb.Allow()
	}
	assert.False(t, tb.Allow())

	// Wait for refill (100 tokens/sec = 1 token per 10ms)
	time.Sleep(50 * time.Millisecond)

	// Should have refilled some tokens
	assert.True(t, tb.Allow(), "should have tokens after waiting")
}

func TestTokenBucket_DoesNotExceedBurst(t *testing.T) {
	tb := newTokenBucket(1000, 5) // High rate but low burst

	// Wait to accumulate tokens
	time.Sleep(100 * time.Millisecond)

	// Should still be capped at burst size
	allowed := 0
	for range 10 {
		if tb.Allow() {
			allowed++
		}
	}

	assert.LessOrEqual(t, allowed, 5, "should not exceed burst capacity")
}

func TestTokenBucket_ZeroRate(t *testing.T) {
	tb := newTokenBucket(0, 0)

	// Should deny immediately with zero tokens and zero refill
	assert.False(t, tb.Allow())

	time.Sleep(50 * time.Millisecond)
	assert.False(t, tb.Allow(), "should still deny with zero refill rate")
}

func TestTokenBucket_HighRate(t *testing.T) {
	tb := newTokenBucket(100000, 1000) // Very high rate

	for range 1000 {
		assert.True(t, tb.Allow())
	}
}

func TestTokenBucket_ConcurrentAccess(t *testing.T) {
	tb := newTokenBucket(1000, 100)

	var wg sync.WaitGroup
	allowed := make([]int, 10)

	wg.Add(10)
	for g := range 10 {
		go func(id int) {
			defer wg.Done()
			for range 50 {
				if tb.Allow() {
					allowed[id]++
				}
			}
		}(g)
	}

	wg.Wait()

	total := 0
	for _, a := range allowed {
		total += a
	}

	// Total allowed should not exceed burst + what was refilled
	// With 10 goroutines x 50 calls = 500 total calls,
	// burst=100, plus some refill during execution
	assert.GreaterOrEqual(t, total, 100, "should allow at least burst capacity")
	assert.LessOrEqual(t, total, 500, "should not exceed total calls")
}

// --- Connection Tests ---

func TestConnection_New(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	require.NotNil(t, conn)
	assert.Equal(t, meta, conn.Metadata())
	assert.Equal(t, "p1", conn.Metadata().ProfileID)
	assert.Equal(t, "d1", conn.Metadata().DeviceID)
}

func TestConnection_Dispatch(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	evt := &chatv1.StreamResponse{Id: "evt1"}
	ok := conn.Dispatch(evt)
	assert.True(t, ok)
}

func TestConnection_DispatchAndConsume(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	evt := &chatv1.StreamResponse{Id: "evt1"}
	ok := conn.Dispatch(evt)
	require.True(t, ok)

	ctx := context.Background()
	received := conn.ConsumeDispatch(ctx)
	require.NotNil(t, received)
	assert.Equal(t, "evt1", received.GetId())
}

func TestConnection_ConsumeDispatch_CancelledContext(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	received := conn.ConsumeDispatch(ctx)
	assert.Nil(t, received)
}

func TestConnection_DispatchFull(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	// Fill the channel
	for i := range dispatchChannelSize {
		evt := &chatv1.StreamResponse{Id: fmt.Sprintf("evt%d", i)}
		ok := conn.Dispatch(evt)
		require.True(t, ok, "dispatch %d should succeed", i)
	}

	// Next dispatch should fail (with timeout)
	evt := &chatv1.StreamResponse{Id: "overflow"}
	ok := conn.Dispatch(evt)
	assert.False(t, ok, "dispatch should fail when channel is full")
}

func TestConnection_AllowInbound(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	// Should allow up to burst
	for range rateLimitBurst {
		assert.True(t, conn.AllowInbound())
	}

	// Should deny after burst exhausted
	assert.False(t, conn.AllowInbound())
}

func TestConnection_RateLimitedCount(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta).(*connection)

	// Exhaust burst
	for range rateLimitBurst {
		conn.AllowInbound()
	}

	assert.Equal(t, uint64(0), conn.RateLimitedCount())

	// These should be rate limited
	conn.AllowInbound()
	conn.AllowInbound()
	conn.AllowInbound()

	assert.Equal(t, uint64(3), conn.RateLimitedCount())
}

func TestConnection_DispatchedMessages(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta).(*connection)

	for i := range 5 {
		conn.Dispatch(&chatv1.StreamResponse{Id: fmt.Sprintf("evt%d", i)})
	}

	assert.Equal(t, uint64(5), conn.DispatchedMessages())
}

func TestConnection_DroppedMessages(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta).(*connection)

	// Fill the buffer
	for range dispatchChannelSize {
		conn.Dispatch(&chatv1.StreamResponse{Id: "fill"})
	}

	// This should be dropped (after timeout)
	conn.Dispatch(&chatv1.StreamResponse{Id: "drop"})

	assert.Equal(t, uint64(1), conn.DroppedMessages())
}

func TestConnection_ChannelUtilization(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta).(*connection)

	assert.InDelta(t, 0.0, conn.ChannelUtilization(), 0.001)

	// Fill half the channel
	for range dispatchChannelSize / 2 {
		conn.Dispatch(&chatv1.StreamResponse{Id: "msg"})
	}

	utilization := conn.ChannelUtilization()
	assert.InDelta(t, 0.5, utilization, 0.05)
}

func TestConnection_Close(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	// Close should not panic
	assert.NotPanics(t, func() {
		conn.Close()
	})
}

func TestConnection_LockUnlock(t *testing.T) {
	meta := &Metadata{ProfileID: "p1", DeviceID: "d1"}
	conn := NewConnection(nil, meta)

	// Lock/Unlock should not deadlock
	assert.NotPanics(t, func() {
		conn.Lock()
		_ = conn.Metadata() // Access state under lock
		conn.Unlock()
	})
}

func TestConnection_MetadataKey(t *testing.T) {
	meta := &Metadata{ProfileID: "user123", DeviceID: "device456"}
	conn := NewConnection(nil, meta)

	assert.Equal(t, "user123:device456", conn.Metadata().Key())
}
