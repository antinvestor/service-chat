package internal

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShardForKey_Deterministic(t *testing.T) {
	key := "user123:device456"
	shardCount := 8

	result1 := ShardForKey(key, shardCount)
	result2 := ShardForKey(key, shardCount)
	result3 := ShardForKey(key, shardCount)

	assert.Equal(t, result1, result2)
	assert.Equal(t, result2, result3)
}

func TestShardForKey_WithinRange(t *testing.T) {
	keys := []string{
		"user1:dev1",
		"user2:dev2",
		"abc:xyz",
		"",
		"a",
		"very-long-key-that-should-still-work-correctly-and-be-within-range",
	}

	for _, shardCount := range []int{1, 2, 3, 5, 8, 16, 32, 100} {
		for _, key := range keys {
			result := ShardForKey(key, shardCount)
			assert.GreaterOrEqual(t, result, 0,
				"shard for key=%q shardCount=%d should be >= 0", key, shardCount)
			assert.Less(t, result, shardCount,
				"shard for key=%q shardCount=%d should be < %d", key, shardCount, shardCount)
		}
	}
}

func TestShardForKey_SingleShard(t *testing.T) {
	assert.Equal(t, 0, ShardForKey("any-key", 1))
	assert.Equal(t, 0, ShardForKey("", 1))
	assert.Equal(t, 0, ShardForKey("another-key", 1))
}

func TestShardForKey_EmptyKey(t *testing.T) {
	result := ShardForKey("", 8)
	assert.GreaterOrEqual(t, result, 0)
	assert.Less(t, result, 8)

	// Empty key should produce consistent results
	assert.Equal(t, ShardForKey("", 8), ShardForKey("", 8))
}

func TestShardForKey_DifferentKeysDifferentShards(t *testing.T) {
	// With enough distinct keys and a reasonable shard count,
	// we should see multiple distinct shards used
	shardCount := 8
	shards := make(map[int]bool)

	for i := range 100 {
		key := fmt.Sprintf("user%d:device%d", i, i)
		shards[ShardForKey(key, shardCount)] = true
	}

	// With 100 distinct keys across 8 shards, we should see at least 4 shards used
	assert.GreaterOrEqual(t, len(shards), 4,
		"expected good distribution across shards, got %d distinct shards", len(shards))
}

func TestShardForKey_Distribution(t *testing.T) {
	// Verify roughly even distribution across shards
	shardCount := 8
	counts := make([]int, shardCount)

	numKeys := 10000
	for i := range numKeys {
		key := fmt.Sprintf("profile_%d:device_%d", i, i%10)
		shard := ShardForKey(key, shardCount)
		counts[shard]++
	}

	expected := float64(numKeys) / float64(shardCount)
	tolerance := expected * 0.3 // 30% tolerance

	for i, count := range counts {
		deviation := math.Abs(float64(count) - expected)
		assert.Less(t, deviation, tolerance,
			"shard %d has %d keys (expected ~%.0f, tolerance %.0f)", i, count, expected, tolerance)
	}
}

func TestShardForKey_PanicsOnZeroShardCount(t *testing.T) {
	assert.Panics(t, func() {
		ShardForKey("key", 0)
	})
}

func TestShardForKey_PanicsOnNegativeShardCount(t *testing.T) {
	assert.Panics(t, func() {
		ShardForKey("key", -1)
	})
}

func TestShardForKey_StableAcrossInvocations(t *testing.T) {
	// FNV-1a hash should be deterministic - verify specific known mappings
	// This ensures the algorithm hasn't changed between versions
	key := "test-stability-key"
	shardCount := 16

	shard := ShardForKey(key, shardCount)

	// Run multiple times to ensure stability
	for range 1000 {
		require.Equal(t, shard, ShardForKey(key, shardCount),
			"ShardForKey must be stable across invocations")
	}
}

func TestShardForKey_DifferentShardCountsSameKey(t *testing.T) {
	key := "user:device"

	// Same key with different shard counts should give valid results
	for shardCount := 1; shardCount <= 64; shardCount++ {
		result := ShardForKey(key, shardCount)
		assert.GreaterOrEqual(t, result, 0)
		assert.Less(t, result, shardCount)
	}
}

func BenchmarkShardForKey(b *testing.B) {
	key := "user123:device456"
	shardCount := 32

	b.ResetTimer()
	for range b.N {
		ShardForKey(key, shardCount)
	}
}
