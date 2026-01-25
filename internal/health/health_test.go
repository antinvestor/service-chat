package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/antinvestor/service-chat/internal/health"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockChecker implements the Checker interface for testing.
type mockChecker struct {
	name   string
	result health.CheckResult
	delay  time.Duration
}

func (m *mockChecker) Name() string {
	return m.name
}

func (m *mockChecker) Check(_ context.Context) health.CheckResult {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return m.result
}

func TestHandler_LivenessHandler(t *testing.T) {
	handler := health.NewHandler()

	t.Run("returns healthy status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		handler.LivenessHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response health.HealthResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, health.StatusHealthy, response.Status)
	})

	t.Run("liveness check is lightweight", func(t *testing.T) {
		// Add a slow checker to verify liveness doesn't use it
		handler.AddChecker(&mockChecker{
			name:  "slow_check",
			delay: 5 * time.Second,
		})

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		start := time.Now()
		handler.LivenessHandler(w, req)
		elapsed := time.Since(start)

		// Liveness should return immediately (< 100ms)
		assert.Less(t, elapsed, 100*time.Millisecond)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandler_ReadinessHandler(t *testing.T) {
	t.Run("all checks healthy", func(t *testing.T) {
		handler := health.NewHandler()
		handler.AddChecker(&mockChecker{
			name:   "database",
			result: health.CheckResult{Status: health.StatusHealthy, LatencyMs: 5},
		})
		handler.AddChecker(&mockChecker{
			name:   "cache",
			result: health.CheckResult{Status: health.StatusHealthy, LatencyMs: 2},
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.ReadinessHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response health.HealthResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, health.StatusHealthy, response.Status)
		assert.Len(t, response.Checks, 2)
		assert.Equal(t, health.StatusHealthy, response.Checks["database"].Status)
		assert.Equal(t, health.StatusHealthy, response.Checks["cache"].Status)
	})

	t.Run("degraded when one check degraded", func(t *testing.T) {
		handler := health.NewHandler()
		handler.AddChecker(&mockChecker{
			name:   "database",
			result: health.CheckResult{Status: health.StatusHealthy, LatencyMs: 5},
		})
		handler.AddChecker(&mockChecker{
			name:   "cache",
			result: health.CheckResult{Status: health.StatusDegraded, LatencyMs: 100, Error: "high latency"},
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.ReadinessHandler(w, req)

		// Degraded still returns 200
		assert.Equal(t, http.StatusOK, w.Code)

		var response health.HealthResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, health.StatusDegraded, response.Status)
	})

	t.Run("unhealthy when one check fails", func(t *testing.T) {
		handler := health.NewHandler()
		handler.AddChecker(&mockChecker{
			name:   "database",
			result: health.CheckResult{Status: health.StatusUnhealthy, Error: "connection refused"},
		})
		handler.AddChecker(&mockChecker{
			name:   "cache",
			result: health.CheckResult{Status: health.StatusHealthy, LatencyMs: 2},
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.ReadinessHandler(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)

		var response health.HealthResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, health.StatusUnhealthy, response.Status)
		assert.Equal(t, "connection refused", response.Checks["database"].Error)
	})

	t.Run("no checkers returns healthy", func(t *testing.T) {
		handler := health.NewHandler()

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		handler.ReadinessHandler(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response health.HealthResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)

		assert.Equal(t, health.StatusHealthy, response.Status)
		assert.Empty(t, response.Checks)
	})

	t.Run("checks run concurrently", func(t *testing.T) {
		handler := health.NewHandler()

		// Add multiple slow checkers
		for i := 0; i < 5; i++ {
			handler.AddChecker(&mockChecker{
				name:   "check" + string(rune('A'+i)),
				result: health.CheckResult{Status: health.StatusHealthy},
				delay:  50 * time.Millisecond,
			})
		}

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		w := httptest.NewRecorder()

		start := time.Now()
		handler.ReadinessHandler(w, req)
		elapsed := time.Since(start)

		// If concurrent, should complete in ~50ms, not ~250ms
		assert.Less(t, elapsed, 150*time.Millisecond)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestPingChecker(t *testing.T) {
	t.Run("healthy ping", func(t *testing.T) {
		checker := health.NewPingChecker("test", func(_ context.Context) error {
			return nil
		}, 5*time.Second)

		result := checker.Check(context.Background())

		assert.Equal(t, "test", checker.Name())
		assert.Equal(t, health.StatusHealthy, result.Status)
		assert.Empty(t, result.Error)
		assert.GreaterOrEqual(t, result.LatencyMs, int64(0))
	})

	t.Run("unhealthy ping", func(t *testing.T) {
		checker := health.NewPingChecker("test", func(_ context.Context) error {
			return errors.New("connection refused")
		}, 5*time.Second)

		result := checker.Check(context.Background())

		assert.Equal(t, health.StatusUnhealthy, result.Status)
		assert.Equal(t, "connection refused", result.Error)
	})

	t.Run("timeout", func(t *testing.T) {
		checker := health.NewPingChecker("test", func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		}, 50*time.Millisecond)

		result := checker.Check(context.Background())

		assert.Equal(t, health.StatusUnhealthy, result.Status)
		assert.Contains(t, result.Error, "deadline exceeded")
	})
}

func TestStatus_Constants(t *testing.T) {
	assert.Equal(t, health.Status("healthy"), health.StatusHealthy)
	assert.Equal(t, health.Status("degraded"), health.StatusDegraded)
	assert.Equal(t, health.Status("unhealthy"), health.StatusUnhealthy)
}

func TestCheckResult_JSON(t *testing.T) {
	result := health.CheckResult{
		Status:    health.StatusHealthy,
		LatencyMs: 42,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded health.CheckResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result.Status, decoded.Status)
	assert.Equal(t, result.LatencyMs, decoded.LatencyMs)
}

func TestHealthResponse_JSON(t *testing.T) {
	response := health.HealthResponse{
		Status: health.StatusDegraded,
		Checks: map[string]health.CheckResult{
			"database": {Status: health.StatusHealthy, LatencyMs: 5},
			"cache":    {Status: health.StatusDegraded, LatencyMs: 100, Error: "high latency"},
		},
	}

	data, err := json.Marshal(response)
	require.NoError(t, err)

	var decoded health.HealthResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, response.Status, decoded.Status)
	assert.Len(t, decoded.Checks, 2)
}
