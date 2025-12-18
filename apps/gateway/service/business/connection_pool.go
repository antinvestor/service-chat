package business

import (
	"sync"
	"sync/atomic"
)

// connectionPool manages active connections with atomic operations for high performance.
//
// Design Rationale:
// - Uses atomic operations for size tracking to enable lock-free reads
// - RWMutex allows concurrent reads while protecting writes
// - Pre-allocated map capacity reduces resizing overhead
// - forEach creates snapshot to avoid holding lock during iteration
//
// Performance:
// - add(): O(1) with atomic check before lock acquisition
// - get(): O(1) with read lock only
// - remove(): O(1) with write lock
// - size(): O(1) lock-free atomic read
// - forEach(): O(n) with snapshot to release lock early.
type connectionPool struct {
	mu          sync.RWMutex          // Protects connections map
	connections map[string]Connection // Key: "profileID:deviceID"
	maxSize     int32                 // Maximum pool capacity
	currentSize int32                 // Current size (atomic access)
}

// newConnectionPool creates a connection pool with the specified capacity.
// The map is pre-allocated to maxSize to avoid resizing during operation.
func newConnectionPool(maxSize int32) *connectionPool {
	return &connectionPool{
		connections: make(map[string]Connection, maxSize),
		maxSize:     maxSize,
	}
}

// add inserts a connection into the pool.
// Returns ErrConnectionPoolFull if the pool is at capacity.
// If a connection with the same key exists, it is not replaced.
// Thread-safe: Uses atomic load before acquiring write lock.
func (p *connectionPool) add(conn Connection) error {
	// Fast-path check without lock
	if atomic.LoadInt32(&p.currentSize) >= p.maxSize {
		return ErrConnectionPoolFull
	}

	key := conn.Metadata().Key()
	p.mu.Lock()
	if _, exists := p.connections[key]; !exists {
		p.connections[key] = conn
		atomic.AddInt32(&p.currentSize, 1)
	}
	p.mu.Unlock()
	return nil
}

// get retrieves a connection from the pool.
// Returns the connection and true if found, nil and false otherwise.
// Thread-safe: Uses read lock for concurrent access.
func (p *connectionPool) get(key string) (Connection, bool) {
	p.mu.RLock()
	conn, exists := p.connections[key]
	p.mu.RUnlock()
	return conn, exists
}

// remove deletes a connection from the pool.
// No-op if the connection doesn't exist.
// Thread-safe: Uses write lock.
func (p *connectionPool) remove(key string) {
	p.mu.Lock()
	if _, exists := p.connections[key]; exists {
		delete(p.connections, key)
		atomic.AddInt32(&p.currentSize, -1)
	}
	p.mu.Unlock()
}

// size returns the current number of connections in the pool.
// Thread-safe: Lock-free atomic read.
func (p *connectionPool) size() int32 {
	return atomic.LoadInt32(&p.currentSize)
}

// forEach iterates over all connections in the pool, calling fn for each.
// Creates a snapshot of connections before iteration to avoid holding the lock.
// This prevents deadlocks if fn performs operations that acquire locks.
// Thread-safe: Releases read lock before calling fn.
func (p *connectionPool) forEach(fn func(Connection)) {
	// Create snapshot under read lock
	p.mu.RLock()
	conns := make([]Connection, 0, len(p.connections))
	for _, conn := range p.connections {
		conns = append(conns, conn)
	}
	p.mu.RUnlock()

	// Iterate without holding lock
	for _, conn := range conns {
		fn(conn)
	}
}
