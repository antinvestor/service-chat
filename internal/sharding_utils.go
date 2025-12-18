package internal

import "hash/fnv"

// ShardForKey returns a shard number in range [0, shardCount).
// shardCount must be > 0.
func ShardForKey(key string, shardCount uint32) int {
	if shardCount == 0 {
		panic("shardCount must be > 0")
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(key)) // never returns error
	return int(h.Sum32() % shardCount)
}
