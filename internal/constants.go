package internal

const (
	HeaderPriority  = "priority"
	HeaderProfileID = "profile_id"
	HeaderDeviceID  = "device_id"
	HeaderShardID   = "shard_id"

	// Dead-letter queue headers
	HeaderDLQOriginalQueue = "dlq_original_queue"
	HeaderDLQErrorMessage  = "dlq_error_message"
)
