// Package health provides health check functionality for Kubernetes probes.
package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/pitabwire/frame/cache"
	"github.com/pitabwire/frame/datastore/pool"
)

// Status represents the health status of a component.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// CheckResult represents the result of a single health check.
type CheckResult struct {
	Status    Status `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthResponse is the response format for health check endpoints.
type HealthResponse struct {
	Status Status                 `json:"status"`
	Checks map[string]CheckResult `json:"checks,omitempty"`
}

// Checker is the interface for health check components.
type Checker interface {
	Name() string
	Check(ctx context.Context) CheckResult
}

// Handler manages health checks and provides HTTP handlers.
type Handler struct {
	checkers []Checker
	mu       sync.RWMutex
}

// NewHandler creates a new health check handler.
func NewHandler() *Handler {
	return &Handler{
		checkers: make([]Checker, 0),
	}
}

// AddChecker adds a health checker.
func (h *Handler) AddChecker(checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers = append(h.checkers, checker)
}

// LivenessHandler handles the /healthz endpoint.
// This is a lightweight check - returns 200 if the service is running.
func (h *Handler) LivenessHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthResponse{Status: StatusHealthy})
}

// ReadinessHandler handles the /readyz endpoint.
// This performs full health checks on all registered checkers.
func (h *Handler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h.mu.RLock()
	checkers := h.checkers
	h.mu.RUnlock()

	response := HealthResponse{
		Status: StatusHealthy,
		Checks: make(map[string]CheckResult),
	}

	// Run all checks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, checker := range checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()

			result := c.Check(ctx)

			mu.Lock()
			response.Checks[c.Name()] = result

			// Update overall status based on individual check results
			if result.Status == StatusUnhealthy && response.Status != StatusUnhealthy {
				response.Status = StatusUnhealthy
			} else if result.Status == StatusDegraded && response.Status == StatusHealthy {
				response.Status = StatusDegraded
			}
			mu.Unlock()
		}(checker)
	}

	wg.Wait()

	w.Header().Set("Content-Type", "application/json")

	switch response.Status {
	case StatusHealthy:
		w.WriteHeader(http.StatusOK)
	case StatusDegraded:
		w.WriteHeader(http.StatusOK) // Still return 200 for degraded
	case StatusUnhealthy:
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(response)
}

// DatabaseChecker checks database connectivity.
type DatabaseChecker struct {
	pool    pool.Pool
	timeout time.Duration
}

// NewDatabaseChecker creates a new database health checker.
func NewDatabaseChecker(p pool.Pool, timeout time.Duration) *DatabaseChecker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &DatabaseChecker{
		pool:    p,
		timeout: timeout,
	}
}

// Name returns the checker name.
func (d *DatabaseChecker) Name() string {
	return "database"
}

// Check performs the database health check.
func (d *DatabaseChecker) Check(ctx context.Context) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	start := time.Now()

	db := d.pool.DB(ctx, true)
	sqlDB, err := db.DB()
	if err != nil {
		return CheckResult{
			Status:    StatusUnhealthy,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     err.Error(),
		}
	}

	err = sqlDB.PingContext(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    StatusUnhealthy,
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	// Check connection pool stats for degraded state
	stats := sqlDB.Stats()
	if stats.OpenConnections > 0 && stats.InUse == stats.MaxOpenConnections {
		return CheckResult{
			Status:    StatusDegraded,
			LatencyMs: latency,
			Error:     "connection pool exhausted",
		}
	}

	return CheckResult{
		Status:    StatusHealthy,
		LatencyMs: latency,
	}
}

// CacheChecker checks cache connectivity.
type CacheChecker struct {
	cache   cache.RawCache
	timeout time.Duration
}

// NewCacheChecker creates a new cache health checker.
func NewCacheChecker(c cache.RawCache, timeout time.Duration) *CacheChecker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &CacheChecker{
		cache:   c,
		timeout: timeout,
	}
}

// Name returns the checker name.
func (c *CacheChecker) Name() string {
	return "cache"
}

// Check performs the cache health check.
func (c *CacheChecker) Check(ctx context.Context) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()

	// Try to ping by getting a non-existent key
	// RawCache.Get returns ([]byte, bool, error) - the bool indicates if key was found
	_, _, err := c.cache.Get(ctx, "__health_check__")
	latency := time.Since(start).Milliseconds()

	// Any error indicates a connectivity problem
	if err != nil {
		return CheckResult{
			Status:    StatusUnhealthy,
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	return CheckResult{
		Status:    StatusHealthy,
		LatencyMs: latency,
	}
}

// SQLDBChecker wraps a *sql.DB for health checking.
type SQLDBChecker struct {
	db      *sql.DB
	name    string
	timeout time.Duration
}

// NewSQLDBChecker creates a new SQL database health checker.
func NewSQLDBChecker(db *sql.DB, name string, timeout time.Duration) *SQLDBChecker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &SQLDBChecker{
		db:      db,
		name:    name,
		timeout: timeout,
	}
}

// Name returns the checker name.
func (s *SQLDBChecker) Name() string {
	return s.name
}

// Check performs the health check.
func (s *SQLDBChecker) Check(ctx context.Context) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	start := time.Now()
	err := s.db.PingContext(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    StatusUnhealthy,
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	return CheckResult{
		Status:    StatusHealthy,
		LatencyMs: latency,
	}
}

// PingChecker provides a generic ping-based health check.
type PingChecker struct {
	name    string
	pingFn  func(ctx context.Context) error
	timeout time.Duration
}

// NewPingChecker creates a new ping-based health checker.
func NewPingChecker(name string, pingFn func(ctx context.Context) error, timeout time.Duration) *PingChecker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &PingChecker{
		name:    name,
		pingFn:  pingFn,
		timeout: timeout,
	}
}

// Name returns the checker name.
func (p *PingChecker) Name() string {
	return p.name
}

// Check performs the health check.
func (p *PingChecker) Check(ctx context.Context) CheckResult {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	start := time.Now()
	err := p.pingFn(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return CheckResult{
			Status:    StatusUnhealthy,
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	return CheckResult{
		Status:    StatusHealthy,
		LatencyMs: latency,
	}
}
