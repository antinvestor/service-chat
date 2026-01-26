package business

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestConnection(profileID, deviceID string) Connection {
	return NewConnection(nil, &Metadata{
		ProfileID: profileID,
		DeviceID:  deviceID,
	})
}

func TestConnectionPool_NewPool(t *testing.T) {
	pool := newConnectionPool(100)
	require.NotNil(t, pool)
	assert.Equal(t, int32(0), pool.size())
	assert.Equal(t, int32(100), pool.maxSize)

	// All shards should be initialized
	for i := range poolShardCount {
		assert.NotNil(t, pool.shards[i])
		assert.NotNil(t, pool.shards[i].connections)
	}
}

func TestConnectionPool_Add(t *testing.T) {
	pool := newConnectionPool(100)

	conn := makeTestConnection("user1", "dev1")
	err := pool.add(conn)
	require.NoError(t, err)

	assert.Equal(t, int32(1), pool.size())
}

func TestConnectionPool_AddMultiple(t *testing.T) {
	pool := newConnectionPool(100)

	for i := range 10 {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		err := pool.add(conn)
		require.NoError(t, err)
	}

	assert.Equal(t, int32(10), pool.size())
}

func TestConnectionPool_AddDuplicate(t *testing.T) {
	pool := newConnectionPool(100)

	conn1 := makeTestConnection("user1", "dev1")
	conn2 := makeTestConnection("user1", "dev1")

	err := pool.add(conn1)
	require.NoError(t, err)

	err = pool.add(conn2)
	require.NoError(t, err)

	// Duplicate should not increase size
	assert.Equal(t, int32(1), pool.size())
}

func TestConnectionPool_AddFull(t *testing.T) {
	pool := newConnectionPool(3)

	for i := range 3 {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		err := pool.add(conn)
		require.NoError(t, err)
	}

	// Pool is full
	conn := makeTestConnection("user_extra", "dev_extra")
	err := pool.add(conn)
	assert.ErrorIs(t, err, ErrConnectionPoolFull)
	assert.Equal(t, int32(3), pool.size())
}

func TestConnectionPool_Get(t *testing.T) {
	pool := newConnectionPool(100)

	conn := makeTestConnection("user1", "dev1")
	require.NoError(t, pool.add(conn))

	retrieved, ok := pool.get(conn.Metadata().Key())
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "user1", retrieved.Metadata().ProfileID)
	assert.Equal(t, "dev1", retrieved.Metadata().DeviceID)
}

func TestConnectionPool_GetNonExistent(t *testing.T) {
	pool := newConnectionPool(100)

	retrieved, ok := pool.get("nonexistent:key")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestConnectionPool_GetFromEmptyPool(t *testing.T) {
	pool := newConnectionPool(100)

	retrieved, ok := pool.get("any:key")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestConnectionPool_Remove(t *testing.T) {
	pool := newConnectionPool(100)

	conn := makeTestConnection("user1", "dev1")
	require.NoError(t, pool.add(conn))
	assert.Equal(t, int32(1), pool.size())

	removed := pool.remove(conn.Metadata().Key())
	assert.NotNil(t, removed)
	assert.Equal(t, int32(0), pool.size())

	// Should no longer be retrievable
	_, ok := pool.get(conn.Metadata().Key())
	assert.False(t, ok)
}

func TestConnectionPool_RemoveNonExistent(t *testing.T) {
	pool := newConnectionPool(100)

	removed := pool.remove("nonexistent:key")
	assert.Nil(t, removed)
	assert.Equal(t, int32(0), pool.size())
}

func TestConnectionPool_RemoveFreesCapacity(t *testing.T) {
	pool := newConnectionPool(2)

	conn1 := makeTestConnection("user1", "dev1")
	conn2 := makeTestConnection("user2", "dev2")

	require.NoError(t, pool.add(conn1))
	require.NoError(t, pool.add(conn2))

	// Pool is full
	conn3 := makeTestConnection("user3", "dev3")
	assert.ErrorIs(t, pool.add(conn3), ErrConnectionPoolFull)

	// Remove one
	pool.remove(conn1.Metadata().Key())

	// Now can add
	err := pool.add(conn3)
	assert.NoError(t, err)
	assert.Equal(t, int32(2), pool.size())
}

func TestConnectionPool_ForEach(t *testing.T) {
	pool := newConnectionPool(100)

	expectedKeys := make(map[string]bool)
	for i := range 5 {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		require.NoError(t, pool.add(conn))
		expectedKeys[conn.Metadata().Key()] = true
	}

	visitedKeys := make(map[string]bool)
	pool.forEach(func(c Connection) {
		visitedKeys[c.Metadata().Key()] = true
	})

	assert.Equal(t, expectedKeys, visitedKeys)
}

func TestConnectionPool_ForEachEmpty(t *testing.T) {
	pool := newConnectionPool(100)

	count := 0
	pool.forEach(func(_ Connection) {
		count++
	})

	assert.Equal(t, 0, count)
}

func TestConnectionPool_ShardDistribution(t *testing.T) {
	pool := newConnectionPool(10000)

	// Add many connections
	for i := range 1000 {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		require.NoError(t, pool.add(conn))
	}

	// Check that connections are distributed across shards
	shardsUsed := 0
	for i := range poolShardCount {
		pool.shards[i].mu.RLock()
		if len(pool.shards[i].connections) > 0 {
			shardsUsed++
		}
		pool.shards[i].mu.RUnlock()
	}

	// With 1000 connections across 32 shards, we expect most shards to be used
	assert.GreaterOrEqual(t, shardsUsed, 20,
		"expected connections to be distributed across most shards, got %d of %d", shardsUsed, poolShardCount)
}

func TestConnectionPool_SameKeyAlwaysSameShard(t *testing.T) {
	pool := newConnectionPool(100)

	key := "user123:dev456"
	shard1 := pool.getShard(key)
	shard2 := pool.getShard(key)
	shard3 := pool.getShard(key)

	assert.Same(t, shard1, shard2)
	assert.Same(t, shard2, shard3)
}

func TestConnectionPool_ConcurrentAddRemove(t *testing.T) {
	pool := newConnectionPool(10000)

	var wg sync.WaitGroup
	numGoroutines := 100
	numOpsPerGoroutine := 50

	// Concurrently add connections
	wg.Add(numGoroutines)
	for g := range numGoroutines {
		go func(gID int) {
			defer wg.Done()
			for i := range numOpsPerGoroutine {
				conn := makeTestConnection(
					fmt.Sprintf("user_%d_%d", gID, i),
					fmt.Sprintf("dev_%d_%d", gID, i),
				)
				_ = pool.add(conn)
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, int32(numGoroutines*numOpsPerGoroutine), pool.size())

	// Concurrently remove connections
	wg.Add(numGoroutines)
	for g := range numGoroutines {
		go func(gID int) {
			defer wg.Done()
			for i := range numOpsPerGoroutine {
				key := fmt.Sprintf("user_%d_%d:dev_%d_%d", gID, i, gID, i)
				pool.remove(key)
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, int32(0), pool.size())
}

func TestConnectionPool_ConcurrentAddAndGet(t *testing.T) {
	pool := newConnectionPool(10000)

	var wg sync.WaitGroup
	numOps := 500

	// Writers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range numOps {
			conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
			_ = pool.add(conn)
		}
	}()

	// Readers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range numOps {
			key := fmt.Sprintf("user%d:dev%d", i, i)
			pool.get(key)
		}
	}()

	wg.Wait()

	// Verify all were added
	assert.Equal(t, int32(numOps), pool.size())
}

func TestConnectionPool_ConcurrentForEach(t *testing.T) {
	pool := newConnectionPool(1000)

	for i := range 100 {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		require.NoError(t, pool.add(conn))
	}

	var wg sync.WaitGroup

	// Multiple concurrent forEach calls
	wg.Add(10)
	for range 10 {
		go func() {
			defer wg.Done()
			count := 0
			pool.forEach(func(_ Connection) {
				count++
			})
			assert.Equal(t, 100, count)
		}()
	}

	wg.Wait()
}

func BenchmarkConnectionPool_Add(b *testing.B) {
	pool := newConnectionPool(int32(b.N + 1))
	conns := make([]Connection, b.N)
	for i := range b.N {
		conns[i] = makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
	}

	b.ResetTimer()
	for i := range b.N {
		_ = pool.add(conns[i])
	}
}

func BenchmarkConnectionPool_Get(b *testing.B) {
	pool := newConnectionPool(int32(b.N + 1))
	keys := make([]string, b.N)
	for i := range b.N {
		conn := makeTestConnection(fmt.Sprintf("user%d", i), fmt.Sprintf("dev%d", i))
		keys[i] = conn.Metadata().Key()
		_ = pool.add(conn)
	}

	b.ResetTimer()
	for i := range b.N {
		pool.get(keys[i%len(keys)])
	}
}
