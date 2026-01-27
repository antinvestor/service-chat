package telemetry_test

import (
	"context"
	"testing"

	chattel "github.com/antinvestor/service-chat/internal/telemetry"
)

func TestMetricsInitialization(t *testing.T) {
	ctx := context.Background()

	// Smoke test: increment each metric without panicking
	chattel.MessagesSentCounter.Add(ctx, 1)
	chattel.MessagesDeliveredCounter.Add(ctx, 1)
	chattel.MessagesFailedCounter.Add(ctx, 1)
	chattel.MessagesDeadLetteredCounter.Add(ctx, 1)
	chattel.RoomsCreatedCounter.Add(ctx, 1)
	chattel.RoomsDeletedCounter.Add(ctx, 1)
	chattel.SubscriptionsAddedCounter.Add(ctx, 1)
	chattel.SubscriptionsRemovedCounter.Add(ctx, 1)
	chattel.OutboxEntriesCreatedCounter.Add(ctx, 1)
	chattel.DeliveryQueueProcessedCounter.Add(ctx, 1)
	chattel.DeliveryQueueRetriedCounter.Add(ctx, 1)
	chattel.NotificationsSentCounter.Add(ctx, 1)
	chattel.NotificationsFailedCounter.Add(ctx, 1)
	chattel.EventFanoutCounter.Add(ctx, 1)

	// Verify histogram can record
	chattel.DeliveryLatencyHistogram.Record(ctx, 42.0)
}

func TestTracersInitialization(t *testing.T) {
	ctx := context.Background()

	// Smoke test: start and end spans without panicking
	ctx1, span1 := chattel.MessageTracer.Start(ctx, "test")
	chattel.MessageTracer.End(ctx1, span1, nil)

	ctx2, span2 := chattel.RoomTracer.Start(ctx, "test")
	chattel.RoomTracer.End(ctx2, span2, nil)

	ctx3, span3 := chattel.DeliveryTracer.Start(ctx, "test")
	chattel.DeliveryTracer.End(ctx3, span3, nil)

	ctx4, span4 := chattel.EventTracer.Start(ctx, "test")
	chattel.EventTracer.End(ctx4, span4, nil)
}
