package business

import (
	"hash/maphash"
	"sync"
	"sync/atomic"
)

const (
	// poolShardCount is the number of shards for the connection pool.
	// Must be a power of 2 for efficient modulo operation.
	poolShardCount = 32
)

// poolShard represents a single shard of the connection pool.
type poolShard struct {
	mu          sync.RWMutex
	connections map[string]Connection
}

// connectionPool manages active connections with sharding for high concurrency.
//
// Design Rationale:
// - Sharding reduces lock contention by distributing connections across 32 shards
// - Each shard has its own RWMutex, allowing parallel operations on different shards
// - Uses atomic operations for global size tracking (lock-free reads)
// - maphash for zero-allocation shard selection (pre-seeded)
//
// Performance:
// - add(): O(1) with 1/32 lock contention probability
// - get(): O(1) with read lock on single shard
// - remove(): O(1) with write lock on single shard
// - size(): O(1) lock-free atomic read
// - forEach(): O(n) with per-shard snapshots.
type connectionPool struct {
	shards      [poolShardCount]*poolShard
	hashSeed    maphash.Seed // Pre-seeded hasher for zero-allocation hashing
	maxSize     int32
	currentSize int32 // Atomic access
}

// newConnectionPool creates a sharded connection pool with the specified capacity.
func newConnectionPool(maxSize int32) *connectionPool {
	pool := &connectionPool{
		maxSize:  maxSize,
		hashSeed: maphash.MakeSeed(), // Initialize seed once at pool creation
	}

	// Pre-allocate each shard with proportional capacity
	const minShardCapacity = 64
	shardCapacity := int(maxSize) / poolShardCount
	if shardCapacity < minShardCapacity {
		shardCapacity = minShardCapacity
	}

	for i := range poolShardCount {
		pool.shards[i] = &poolShard{
			connections: make(map[string]Connection, shardCapacity),
		}
	}

	return pool
}

// getShard returns the shard for a given key using maphash (zero-allocation).
func (p *connectionPool) getShard(key string) *poolShard {
	// maphash.String is inlined by the compiler and performs no allocations
	h := maphash.String(p.hashSeed, key)
	return p.shards[h&(poolShardCount-1)]
}

// add inserts a connection into the pool.
// Returns ErrConnectionPoolFull if the pool is at capacity.
// If a connection with the same key exists, it is not replaced.
// Thread-safe: Uses atomic load before acquiring shard write lock.
func (p *connectionPool) add(conn Connection) error {
	// Fast-path check without lock
	if atomic.LoadInt32(&p.currentSize) >= p.maxSize {
		return ErrConnectionPoolFull
	}

	key := conn.Metadata().Key()
	shard := p.getShard(key)

	shard.mu.Lock()
	if _, exists := shard.connections[key]; !exists {
		shard.connections[key] = conn
		atomic.AddInt32(&p.currentSize, 1)
	}
	shard.mu.Unlock()
	return nil
}

// get retrieves a connection from the pool.
// Returns the connection and true if found, nil and false otherwise.
// Thread-safe: Uses read lock on single shard only.
func (p *connectionPool) get(key string) (Connection, bool) {
	shard := p.getShard(key)

	shard.mu.RLock()
	conn, exists := shard.connections[key]
	shard.mu.RUnlock()
	return conn, exists
}

// remove deletes a connection from the pool.
// No-op if the connection doesn't exist.
// Thread-safe: Uses write lock on single shard only.
func (p *connectionPool) remove(key string) {
	shard := p.getShard(key)

	shard.mu.Lock()
	if _, exists := shard.connections[key]; exists {
		delete(shard.connections, key)
		atomic.AddInt32(&p.currentSize, -1)
	}
	shard.mu.Unlock()
}

// size returns the current number of connections in the pool.
// Thread-safe: Lock-free atomic read.
func (p *connectionPool) size() int32 {
	return atomic.LoadInt32(&p.currentSize)
}

// forEach iterates over all connections in the pool, calling fn for each.
// Creates snapshots per shard to minimize lock contention.
// Thread-safe: Releases each shard's read lock before calling fn.
func (p *connectionPool) forEach(fn func(Connection)) {
	// Collect all connections from all shards
	var allConns []Connection

	for i := range poolShardCount {
		shard := p.shards[i]
		shard.mu.RLock()
		for _, conn := range shard.connections {
			allConns = append(allConns, conn)
		}
		shard.mu.RUnlock()
	}

	// Iterate without holding any locks
	for _, conn := range allConns {
		fn(conn)
	}
}
