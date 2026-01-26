package telemetry_test

import (
	"context"
	"testing"

	chattel "github.com/antinvestor/service-chat/internal/telemetry"
)

func TestMetricsInitialization(t *testing.T) {
	ctx := context.Background()

	// Verify all counters can be incremented without panicking
	counters := []struct {
		name    string
		counter interface{ Add(context.Context, int64, ...interface{ applyMeasurementOption() }) }
	}{
		// Can't directly test metric.Int64Counter.Add due to interface constraints,
		// so we just call them and verify no panic.
	}
	_ = counters

	// Smoke test: increment each metric
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

	// Verify all tracers can create spans without panicking
	tracers := []struct {
		name   string
		tracer interface {
			Start(context.Context, string, ...interface{ applySpanStartOption() }) (context.Context, interface{ End(...interface{ applySpanEndOption() }) })
		}
	}{}
	_ = tracers

	// Smoke test: start and end spans
	ctx1, span1 := chattel.MessageTracer.Start(ctx, "test")
	chattel.MessageTracer.End(ctx1, span1, nil)

	ctx2, span2 := chattel.RoomTracer.Start(ctx, "test")
	chattel.RoomTracer.End(ctx2, span2, nil)

	ctx3, span3 := chattel.DeliveryTracer.Start(ctx, "test")
	chattel.DeliveryTracer.End(ctx3, span3, nil)

	ctx4, span4 := chattel.EventTracer.Start(ctx, "test")
	chattel.EventTracer.End(ctx4, span4, nil)
}
