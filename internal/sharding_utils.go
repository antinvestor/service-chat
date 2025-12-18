package internal

// ShardForKey deterministically maps a string key to a shard in [0, shardCount).
// It is allocation-free, stable across restarts, and safe for hot paths.
//
// shardCount must be > 0.
func ShardForKey(key string, shardCount int) int {
	if shardCount <= 0 {
		panic("shardCount must be > 0")
	}

	// FNV-1a 32-bit
	var hash uint32 = 2166136261
	for i := range len(key) {
		hash ^= uint32(key[i])
		hash *= 16777619
	}

	return int(hash) % shardCount
}
