package queues

import (
	"context"
	"fmt"
	"maps"

	eventsv1 "buf.build/gen/go/antinvestor/chat/protocolbuffers/go/events/v1"
	"github.com/antinvestor/service-chat/apps/default/config"
	"github.com/antinvestor/service-chat/internal"
	"github.com/pitabwire/frame/queue"
	"github.com/pitabwire/util"
)

// DeadLetterPublisher publishes failed deliveries to the dead-letter queue
// when they exceed the maximum retry count.
type DeadLetterPublisher struct {
	cfg  *config.ChatConfig
	qMan queue.Manager
}

// NewDeadLetterPublisher creates a new dead-letter queue publisher.
func NewDeadLetterPublisher(cfg *config.ChatConfig, qMan queue.Manager) *DeadLetterPublisher {
	return &DeadLetterPublisher{
		cfg:  cfg,
		qMan: qMan,
	}
}

// ShouldDeadLetter returns true if the delivery has exceeded the max retry count.
func (dlp *DeadLetterPublisher) ShouldDeadLetter(retryCount int32) bool {
	return int(retryCount) >= dlp.cfg.MaxDeliveryRetries
}

// Publish sends a failed delivery message to the dead-letter queue with
// error context headers for diagnostics.
func (dlp *DeadLetterPublisher) Publish(
	ctx context.Context,
	msg any,
	originalQueue string,
	errMsg string,
	headers map[string]string,
) error {
	topic, err := dlp.qMan.GetPublisher(dlp.cfg.QueueDeadLetterName)
	if err != nil {
		return fmt.Errorf("failed to get dead-letter publisher: %w", err)
	}

	// Add DLQ context to headers
	dlqHeaders := make(map[string]string, len(headers)+2)
	maps.Copy(dlqHeaders, headers)
	dlqHeaders[internal.HeaderDLQOriginalQueue] = originalQueue
	dlqHeaders[internal.HeaderDLQErrorMessage] = errMsg

	if pubErr := topic.Publish(ctx, msg, dlqHeaders); pubErr != nil {
		util.Log(ctx).WithError(pubErr).
			WithField("original_queue", originalQueue).
			Error("failed to publish to dead-letter queue")
		return pubErr
	}

	util.Log(ctx).
		WithField("original_queue", originalQueue).
		WithField("error", errMsg).
		Warn("delivery moved to dead-letter queue after max retries exceeded")

	return nil
}

// RetryOrDeadLetter increments the retry count and republishes the delivery,
// or sends it to the dead-letter queue if max retries have been exceeded.
func RetryOrDeadLetter(
	ctx context.Context,
	qMan queue.Manager,
	dlp *DeadLetterPublisher,
	queueName string,
	delivery *eventsv1.Delivery,
	headers map[string]string,
	originalErr error,
) error {
	delivery.RetryCount++

	if dlp != nil && dlp.ShouldDeadLetter(delivery.GetRetryCount()) {
		return dlp.Publish(ctx, delivery, queueName, originalErr.Error(), headers)
	}

	// Republish to the same queue for retry
	topic, err := qMan.GetPublisher(queueName)
	if err != nil {
		util.Log(ctx).WithError(err).Error("failed to get publisher for retry")
		return err
	}

	if pubErr := topic.Publish(ctx, delivery, headers); pubErr != nil {
		util.Log(ctx).WithError(pubErr).Error("failed to republish for retry")
		return pubErr
	}

	util.Log(ctx).WithField("retry_count", delivery.GetRetryCount()).
		Debug("delivery republished for retry")
	return nil
}
