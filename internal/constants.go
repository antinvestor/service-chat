package internal

const (
	HeaderPriority  = "priority"
	HeaderProfileID = "profile_id"
	HeaderDeviceID  = "device_id"
	HeaderShardID   = "shard_id"

	// HeaderDLQOriginalQueue is the header key for the original queue name in dead-letter messages.
	HeaderDLQOriginalQueue = "dlq_original_queue"
	// HeaderDLQErrorMessage is the header key for the error message in dead-letter messages.
	HeaderDLQErrorMessage = "dlq_error_message"
)
