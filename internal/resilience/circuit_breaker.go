package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state.
type State int32

const (
	StateClosed   State = iota // Normal operation, tracking failures
	StateOpen                  // Failing fast, not calling service
	StateHalfOpen              // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open and rejects the request.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Settings configures a CircuitBreaker.
type Settings struct {
	// Name identifies this circuit breaker for logging.
	Name string

	// MaxFailures is the number of consecutive failures before the circuit opens.
	MaxFailures int64

	// ResetTimeout is how long the circuit stays open before transitioning to half-open.
	ResetTimeout time.Duration

	// HalfOpenMaxRequests is the number of successful requests in half-open
	// state before the circuit closes again.
	HalfOpenMaxRequests int64

	// OnStateChange is called when the circuit breaker changes state.
	OnStateChange func(name string, from, to State)
}

// DefaultSettings returns sensible defaults for a circuit breaker.
func DefaultSettings(name string) Settings {
	return Settings{
		Name:                name,
		MaxFailures:         5,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern to prevent cascading
// failures when external services become unavailable.
type CircuitBreaker struct {
	settings Settings

	mu              sync.Mutex
	state           State
	failures        int64
	successes       int64
	lastStateChange time.Time

	// Metrics (atomic for lock-free reads)
	totalRequests  atomic.Int64
	totalRejected  atomic.Int64
	totalSuccesses atomic.Int64
	totalFailures  atomic.Int64
}

// NewCircuitBreaker creates a new circuit breaker with the given settings.
func NewCircuitBreaker(settings Settings) *CircuitBreaker {
	if settings.MaxFailures <= 0 {
		settings.MaxFailures = 5
	}
	if settings.ResetTimeout <= 0 {
		settings.ResetTimeout = 30 * time.Second
	}
	if settings.HalfOpenMaxRequests <= 0 {
		settings.HalfOpenMaxRequests = 3
	}

	return &CircuitBreaker{
		settings:        settings,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Execute runs the given function through the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open and the request is rejected.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.totalRequests.Add(1)

	if !cb.allowRequest() {
		cb.totalRejected.Add(1)
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentState()
}

// Name returns the circuit breaker name.
func (cb *CircuitBreaker) Name() string {
	return cb.settings.Name
}

// Metrics returns a snapshot of circuit breaker metrics.
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.Lock()
	state := cb.currentState()
	failures := cb.failures
	cb.mu.Unlock()

	return CircuitBreakerMetrics{
		Name:               cb.settings.Name,
		State:              state,
		TotalRequests:      cb.totalRequests.Load(),
		TotalRejected:      cb.totalRejected.Load(),
		TotalSuccesses:     cb.totalSuccesses.Load(),
		TotalFailures:      cb.totalFailures.Load(),
		ConsecutiveFailures: failures,
	}
}

// CircuitBreakerMetrics contains circuit breaker statistics.
type CircuitBreakerMetrics struct {
	Name                string
	State               State
	TotalRequests       int64
	TotalRejected       int64
	TotalSuccesses      int64
	TotalFailures       int64
	ConsecutiveFailures int64
}

// currentState returns the effective state, accounting for timeout transitions.
// Must be called with cb.mu held.
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && time.Since(cb.lastStateChange) >= cb.settings.ResetTimeout {
		cb.setState(StateHalfOpen)
	}
	return cb.state
}

// allowRequest determines if a request should be allowed through.
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		// Allow limited requests in half-open state
		return cb.successes < cb.settings.HalfOpenMaxRequests
	default:
		return true
	}
}

// recordSuccess records a successful execution.
func (cb *CircuitBreaker) recordSuccess() {
	cb.totalSuccesses.Add(1)
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.settings.HalfOpenMaxRequests {
			cb.setState(StateClosed)
		}
	}
}

// recordFailure records a failed execution.
func (cb *CircuitBreaker) recordFailure() {
	cb.totalFailures.Add(1)
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.currentState() {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.settings.MaxFailures {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.setState(StateOpen)
	}
}

// setState transitions to a new state.
// Must be called with cb.mu held.
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.failures = 0
	cb.successes = 0
	cb.lastStateChange = time.Now()

	if cb.settings.OnStateChange != nil {
		cb.settings.OnStateChange(cb.settings.Name, oldState, newState)
	}
}
