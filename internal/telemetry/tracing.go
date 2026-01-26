package telemetry

import (
	"github.com/pitabwire/frame/telemetry"
)

// Service tracers for different components.
//
//nolint:gochecknoglobals // OpenTelemetry tracers must be global for instrumentation
var (
	MessageTracer  = telemetry.NewTracer("chat.message")
	RoomTracer     = telemetry.NewTracer("chat.room")
	DeliveryTracer = telemetry.NewTracer("chat.delivery")
	EventTracer    = telemetry.NewTracer("chat.event")
)
