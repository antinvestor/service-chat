package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfig_ValidateSharding_Valid(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     0,
		TotalShards: 4,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}

func TestGatewayConfig_ValidateSharding_LastShard(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     3,
		TotalShards: 4,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}

func TestGatewayConfig_ValidateSharding_SingleShard(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     0,
		TotalShards: 1,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}

func TestGatewayConfig_ValidateSharding_ShardIDExceedsTotalShards(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     4,
		TotalShards: 4,
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_ID (4) must be < TOTAL_SHARDS (4)")
}

func TestGatewayConfig_ValidateSharding_ShardIDMuchLarger(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     100,
		TotalShards: 4,
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_ID (100) must be < TOTAL_SHARDS (4)")
}

func TestGatewayConfig_ValidateSharding_NegativeShardID(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     -1,
		TotalShards: 4,
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_ID must be >= 0")
}

func TestGatewayConfig_ValidateSharding_ZeroTotalShards(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     0,
		TotalShards: 0,
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TOTAL_SHARDS must be > 0")
}

func TestGatewayConfig_ValidateSharding_NegativeTotalShards(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     0,
		TotalShards: -1,
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TOTAL_SHARDS must be > 0")
}

func TestGatewayConfig_ValidateSharding_DefaultValues(t *testing.T) {
	// Default values from envDefault tags: ShardID=0, TotalShards=1
	cfg := GatewayConfig{
		ShardID:     0,
		TotalShards: 1,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err, "default config values should be valid")
}

func TestGatewayConfig_ValidateSharding_LargeScale(t *testing.T) {
	cfg := GatewayConfig{
		ShardID:     63,
		TotalShards: 64,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}
