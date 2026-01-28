//nolint:testpackage // tests access unexported settings fields
package resilience

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errService = errors.New("service unavailable")

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(Settings{Name: "test"})

	assert.Equal(t, "test", cb.Name())
	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, int64(5), cb.settings.MaxFailures)
	assert.Equal(t, 30*time.Second, cb.settings.ResetTimeout)
	assert.Equal(t, int64(3), cb.settings.HalfOpenMaxRequests)
}

func TestNewCircuitBreaker_CustomSettings(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:                "custom",
		MaxFailures:         10,
		ResetTimeout:        5 * time.Second,
		HalfOpenMaxRequests: 1,
	})

	assert.Equal(t, int64(10), cb.settings.MaxFailures)
	assert.Equal(t, 5*time.Second, cb.settings.ResetTimeout)
	assert.Equal(t, int64(1), cb.settings.HalfOpenMaxRequests)
}

func TestNewCircuitBreaker_InvalidSettings(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		MaxFailures:         -1,
		ResetTimeout:        -1,
		HalfOpenMaxRequests: 0,
	})

	// Should use defaults for invalid values
	assert.Equal(t, int64(5), cb.settings.MaxFailures)
	assert.Equal(t, 30*time.Second, cb.settings.ResetTimeout)
	assert.Equal(t, int64(3), cb.settings.HalfOpenMaxRequests)
}

func TestCircuitBreaker_ClosedState_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultSettings("test"))

	err := cb.Execute(func() error { return nil })
	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_ClosedState_FailureBelowThreshold(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:        "test",
		MaxFailures: 3,
	})

	// Two failures - should stay closed
	for range 2 {
		err := cb.Execute(func() error { return errService })
		require.ErrorIs(t, err, errService)
	}

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:        "test",
		MaxFailures: 3,
	})

	for range 3 {
		_ = cb.Execute(func() error { return errService })
	}

	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_OpenState_RejectsRequests(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:         "test",
		MaxFailures:  1,
		ResetTimeout: time.Hour, // Won't expire during test
	})

	// Trip the circuit
	_ = cb.Execute(func() error { return errService })
	assert.Equal(t, StateOpen, cb.State())

	// Subsequent requests should be rejected
	err := cb.Execute(func() error { return nil })
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:        "test",
		MaxFailures: 3,
	})

	// Two failures
	_ = cb.Execute(func() error { return errService })
	_ = cb.Execute(func() error { return errService })

	// One success resets the counter
	_ = cb.Execute(func() error { return nil })

	// Two more failures - should not open (counter was reset)
	_ = cb.Execute(func() error { return errService })
	_ = cb.Execute(func() error { return errService })

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:         "test",
		MaxFailures:  1,
		ResetTimeout: 10 * time.Millisecond,
	})

	// Trip the circuit
	_ = cb.Execute(func() error { return errService })
	assert.Equal(t, StateOpen, cb.State())

	// Wait for reset timeout
	time.Sleep(20 * time.Millisecond)

	assert.Equal(t, StateHalfOpen, cb.State())
}

func TestCircuitBreaker_HalfOpen_ClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:                "test",
		MaxFailures:         1,
		ResetTimeout:        10 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	})

	// Trip the circuit
	_ = cb.Execute(func() error { return errService })

	// Wait for half-open
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Successful requests in half-open close the circuit
	_ = cb.Execute(func() error { return nil })
	_ = cb.Execute(func() error { return nil })

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_HalfOpen_ReopensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:                "test",
		MaxFailures:         1,
		ResetTimeout:        10 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	})

	// Trip the circuit
	_ = cb.Execute(func() error { return errService })

	// Wait for half-open
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, StateHalfOpen, cb.State())

	// One success followed by a failure reopens
	_ = cb.Execute(func() error { return nil })
	_ = cb.Execute(func() error { return errService })

	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_Metrics(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:        "test-metrics",
		MaxFailures: 5,
	})

	// Some successes
	_ = cb.Execute(func() error { return nil })
	_ = cb.Execute(func() error { return nil })

	// Some failures
	_ = cb.Execute(func() error { return errService })

	metrics := cb.Metrics()
	assert.Equal(t, "test-metrics", metrics.Name)
	assert.Equal(t, StateClosed, metrics.State)
	assert.Equal(t, int64(3), metrics.TotalRequests)
	assert.Equal(t, int64(0), metrics.TotalRejected)
	assert.Equal(t, int64(2), metrics.TotalSuccesses)
	assert.Equal(t, int64(1), metrics.TotalFailures)
	assert.Equal(t, int64(1), metrics.ConsecutiveFailures)
}

func TestCircuitBreaker_Metrics_WithRejected(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:         "test",
		MaxFailures:  1,
		ResetTimeout: time.Hour,
	})

	// Trip the circuit
	_ = cb.Execute(func() error { return errService })

	// Try rejected request
	_ = cb.Execute(func() error { return nil })

	metrics := cb.Metrics()
	assert.Equal(t, int64(2), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.TotalRejected)
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var transitions []struct{ from, to State }
	var mu sync.Mutex
	transitionCh := make(chan struct{}, 10)

	cb := NewCircuitBreaker(Settings{
		Name:         "test",
		MaxFailures:  2,
		ResetTimeout: 10 * time.Millisecond,
		OnStateChange: func(_ string, from, to State) {
			mu.Lock()
			transitions = append(transitions, struct{ from, to State }{from, to})
			mu.Unlock()
			transitionCh <- struct{}{}
		},
	})

	// Trip the circuit: closed -> open
	_ = cb.Execute(func() error { return errService })
	_ = cb.Execute(func() error { return errService })

	// Wait for first callback (closed -> open)
	<-transitionCh

	// Wait for half-open: open -> half-open
	time.Sleep(20 * time.Millisecond)
	_ = cb.State() // Triggers transition check

	// Wait for second callback (open -> half-open)
	<-transitionCh

	mu.Lock()
	require.Len(t, transitions, 2)
	assert.Equal(t, StateClosed, transitions[0].from)
	assert.Equal(t, StateOpen, transitions[0].to)
	assert.Equal(t, StateOpen, transitions[1].from)
	assert.Equal(t, StateHalfOpen, transitions[1].to)
	mu.Unlock()
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:        "concurrent",
		MaxFailures: 100, // High threshold so we don't trip
	})

	var wg sync.WaitGroup
	const goroutines = 50
	const iterations = 100

	for range goroutines {
		wg.Go(func() {
			for range iterations {
				_ = cb.Execute(func() error { return nil })
			}
		})
	}

	wg.Wait()

	metrics := cb.Metrics()
	assert.Equal(t, int64(goroutines*iterations), metrics.TotalRequests)
	assert.Equal(t, int64(goroutines*iterations), metrics.TotalSuccesses)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_ConcurrentFailures(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:         "concurrent-fail",
		MaxFailures:  5,
		ResetTimeout: time.Hour,
	})

	var wg sync.WaitGroup
	const goroutines = 20

	for range goroutines {
		wg.Go(func() {
			_ = cb.Execute(func() error { return errService })
		})
	}

	wg.Wait()

	// Circuit should be open after enough failures
	assert.Equal(t, StateOpen, cb.State())
}

func TestState_String(t *testing.T) {
	assert.Equal(t, "closed", StateClosed.String())
	assert.Equal(t, "open", StateOpen.String())
	assert.Equal(t, "half-open", StateHalfOpen.String())
	assert.Equal(t, "unknown", State(99).String())
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings("my-service")

	assert.Equal(t, "my-service", s.Name)
	assert.Equal(t, int64(5), s.MaxFailures)
	assert.Equal(t, 30*time.Second, s.ResetTimeout)
	assert.Equal(t, int64(3), s.HalfOpenMaxRequests)
	assert.Nil(t, s.OnStateChange)
}

func TestCircuitBreaker_FullCycle(t *testing.T) {
	cb := NewCircuitBreaker(Settings{
		Name:                "full-cycle",
		MaxFailures:         2,
		ResetTimeout:        10 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	// Phase 1: Closed - normal operation
	assert.Equal(t, StateClosed, cb.State())
	require.NoError(t, cb.Execute(func() error { return nil }))

	// Phase 2: Trip to open
	_ = cb.Execute(func() error { return errService })
	_ = cb.Execute(func() error { return errService })
	assert.Equal(t, StateOpen, cb.State())

	// Phase 3: Requests rejected
	err := cb.Execute(func() error { return nil })
	require.ErrorIs(t, err, ErrCircuitOpen)

	// Phase 4: Wait for half-open
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, StateHalfOpen, cb.State())

	// Phase 5: Recover
	require.NoError(t, cb.Execute(func() error { return nil }))
	assert.Equal(t, StateClosed, cb.State())

	// Phase 6: Normal again
	assert.NoError(t, cb.Execute(func() error { return nil }))
}

func TestErrCircuitOpen(t *testing.T) {
	assert.Equal(t, "circuit breaker is open", ErrCircuitOpen.Error())
}
