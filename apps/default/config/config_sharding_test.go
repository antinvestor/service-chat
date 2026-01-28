package config_test

import (
	"strconv"
	"testing"

	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatConfig_ValidateSharding_Valid(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount: 2,
		QueueGatewayEventDeliveryURI: []string{
			"mem://gateway.event.delivery.0",
			"mem://gateway.event.delivery.1",
		},
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}

func TestChatConfig_ValidateSharding_SingleShard(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount: 1,
		QueueGatewayEventDeliveryURI: []string{
			"mem://gateway.event.delivery.0",
		},
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}

func TestChatConfig_ValidateSharding_ZeroShardCount(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount:                   0,
		QueueGatewayEventDeliveryURI: []string{},
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_COUNT must be > 0")
}

func TestChatConfig_ValidateSharding_NegativeShardCount(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount:                   -1,
		QueueGatewayEventDeliveryURI: []string{},
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_COUNT must be > 0")
}

func TestChatConfig_ValidateSharding_URIMismatch_TooFew(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount: 3,
		QueueGatewayEventDeliveryURI: []string{
			"mem://gateway.event.delivery.0",
			"mem://gateway.event.delivery.1",
		},
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_COUNT (3) must match")
	assert.Contains(t, err.Error(), "2")
}

func TestChatConfig_ValidateSharding_URIMismatch_TooMany(t *testing.T) {
	cfg := config.ChatConfig{
		ShardCount: 1,
		QueueGatewayEventDeliveryURI: []string{
			"mem://gateway.event.delivery.0",
			"mem://gateway.event.delivery.1",
			"mem://gateway.event.delivery.2",
		},
	}

	err := cfg.ValidateSharding()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHARD_COUNT (1) must match")
	assert.Contains(t, err.Error(), "3")
}

func TestChatConfig_ValidateSharding_DefaultConfig(t *testing.T) {
	// Default envDefault values: ShardCount=1, URIs has 2 entries
	// This is actually a mismatch in the defaults - the test documents this
	cfg := config.ChatConfig{
		ShardCount: 1,
		QueueGatewayEventDeliveryURI: []string{
			"mem://gateway.event.delivery.0",
			"mem://gateway.event.delivery.1",
		},
	}

	err := cfg.ValidateSharding()
	require.Error(t, err, "default config has ShardCount=1 but 2 URIs, should fail validation")
}

func TestChatConfig_ValidateSharding_LargeShardCount(t *testing.T) {
	uris := make([]string, 16)
	for i := range uris {
		uris[i] = "mem://gateway.event.delivery." + strconv.Itoa(i)
	}

	cfg := config.ChatConfig{
		ShardCount:                   16,
		QueueGatewayEventDeliveryURI: uris,
	}

	err := cfg.ValidateSharding()
	require.NoError(t, err)
}
