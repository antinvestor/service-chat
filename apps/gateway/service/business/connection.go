package business

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	chatv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/chat/v1"
)

const (
	// dispatchChannelSize is the buffer size for outbound messages.
	// Reduced from 100 to 32 to limit memory usage per connection.
	// At ~1KB avg message size: 32 * 1KB = 32KB max buffer per connection.
	dispatchChannelSize = 32

	// dispatchTimeout is the maximum time to wait when channel is full
	// before applying backpressure.
	dispatchTimeout = 100 * time.Millisecond

	// defaultRateLimit is the default maximum events per second per connection.
	defaultRateLimit = 100

	// rateLimitBurst is the burst capacity for the rate limiter.
	rateLimitBurst = 20
)

// tokenBucket implements a simple token bucket rate limiter.
// It allows bursting up to 'burst' tokens and refills at 'rate' tokens per second.
type tokenBucket struct {
	tokens     float64    // Current tokens available
	maxTokens  float64    // Maximum tokens (burst capacity)
	refillRate float64    // Tokens added per second
	lastRefill time.Time  // Last time tokens were refilled
	mu         sync.Mutex // Protects token state
}

// newTokenBucket creates a new token bucket rate limiter.
func newTokenBucket(ratePerSecond, burst int) *tokenBucket {
	return &tokenBucket{
		tokens:     float64(burst), // Start full
		maxTokens:  float64(burst),
		refillRate: float64(ratePerSecond),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed and consumes a token if so.
// Returns true if the request is allowed, false if rate limited.
func (tb *tokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// Check if we have tokens available
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// connection represents an active edge device connection.
type connection struct {
	metadata     *Metadata
	dispatchChan chan *chatv1.ConnectResponse
	stream       DeviceStream
	mu           sync.RWMutex

	// Rate limiting
	rateLimiter    *tokenBucket  // Token bucket for inbound rate limiting
	rateLimitedCnt atomic.Uint64 // Number of requests rate limited

	// Backpressure metrics
	droppedMsgs  atomic.Uint64 // Number of messages dropped due to slow consumer
	dispatchedMs atomic.Uint64 // Total messages successfully dispatched
}

func (c *connection) ConsumeDispatch(ctx context.Context) *chatv1.ConnectResponse {
	select {
	case <-ctx.Done():
		return nil
	case data := <-c.dispatchChan:
		return data
	}
}

func (c *connection) Lock() {
	c.mu.Lock()
}

func (c *connection) Unlock() {
	c.mu.Unlock()
}

func NewConnection(stream DeviceStream, metadata *Metadata) Connection {
	return &connection{
		metadata:     metadata,
		stream:       stream,
		dispatchChan: make(chan *chatv1.ConnectResponse, dispatchChannelSize),
		rateLimiter:  newTokenBucket(defaultRateLimit, rateLimitBurst),
	}
}

// AllowInbound checks if an inbound request is allowed by the rate limiter.
// Returns true if allowed, false if rate limited.
func (c *connection) AllowInbound() bool {
	if c.rateLimiter == nil {
		return true
	}
	if c.rateLimiter.Allow() {
		return true
	}
	c.rateLimitedCnt.Add(1)
	return false
}

// RateLimitedCount returns the number of requests that were rate limited.
func (c *connection) RateLimitedCount() uint64 {
	return c.rateLimitedCnt.Load()
}

func (c *connection) Metadata() *Metadata {
	return c.metadata
}

func (c *connection) Dispatch(evt *chatv1.ConnectResponse) bool {
	// First try non-blocking send
	select {
	case c.dispatchChan <- evt:
		c.dispatchedMs.Add(1)
		return true
	default:
	}

	// Channel full - apply backpressure with timeout
	// This gives slow consumers a brief window to catch up
	timer := time.NewTimer(dispatchTimeout)
	defer timer.Stop()

	select {
	case c.dispatchChan <- evt:
		c.dispatchedMs.Add(1)
		return true
	case <-timer.C:
		// Timeout expired - connection is too slow
		c.droppedMsgs.Add(1)
		return false
	}
}

// DroppedMessages returns the count of messages dropped due to slow consumer.
func (c *connection) DroppedMessages() uint64 {
	return c.droppedMsgs.Load()
}

// DispatchedMessages returns the count of successfully dispatched messages.
func (c *connection) DispatchedMessages() uint64 {
	return c.dispatchedMs.Load()
}

// ChannelUtilization returns the current channel buffer utilization (0.0 to 1.0).
func (c *connection) ChannelUtilization() float64 {
	return float64(len(c.dispatchChan)) / float64(dispatchChannelSize)
}

func (c *connection) Stream() DeviceStream {
	return c.stream
}

func (c *connection) Close() {
	close(c.dispatchChan)
}
