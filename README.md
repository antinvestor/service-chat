# Real-Time Message Flow Architecture

## Overview

The chat service implements a scalable, queue-based architecture for real-time message delivery to end users. Messages flow from sender → default service → queue → gateway → device streams.

## Complete Flow

```
┌──────────┐
│  Sender  │
│  Device  │
└────┬─────┘
     │ 1. SendEvent (HTTP/gRPC)
     ▼
┌─────────────────────────────────────────────────────────┐
│              DEFAULT SERVICE                             │
│  ┌────────────────────────────────────────────────┐    │
│  │ MessageBusiness.SendEvents()                    │    │
│  │  - Validates room access                        │    │
│  │  - Saves RoomEvent to database                  │    │
│  │  - Emits "room.outbox.logging.event"            │    │
│  └──────────────────┬──────────────────────────────┘    │
│                     ▼                                    │
│  ┌────────────────────────────────────────────────┐    │
│  │ RoomOutboxLoggingQueue (Event Handler)          │    │
│  │  - Gets all active room subscribers             │    │
│  │  - Creates outbox entry for each subscriber     │    │
│  │  - Batch inserts to database                    │    │
│  │  - Emits "outbox.delivery.event" with           │    │
│  │    EventBroadcast (event + targets)             │    │
│  └──────────────────┬──────────────────────────────┘    │
│                     ▼                                    │
│  ┌────────────────────────────────────────────────┐    │
│  │ OutboxDeliveryEventHandler (Event Handler)      │    │
│  │  - Gets full event data from database           │    │
│  │  - For each target (recipient):                 │    │
│  │    - Creates EventDelivery proto                 │    │
│  │    - Publishes to "user.event.delivery" queue   │    │
│  └──────────────────┬──────────────────────────────┘    │
└────────────────────┼──────────────────────────────────┘
                     │
                     ▼
         ┌────────────────────────┐
         │  QUEUE INFRASTRUCTURE  │
         │  (user.event.delivery) │
         └──────────┬──────────────┘
                    │
                    ▼
┌───────────────────────────────────────────────────────────┐
│  EventDeliveryQueueHandler (Queue Subscriber)              │
│   - Queries device service for user's devices             │
│   - For each device:                                      │
│     - Checks if online (TODO: query gateway cache)        │
│     - If online: Publishes to queue with headers:         │
│         * profile_id: {recipient_id}                      │
│         * device_id: {device_id}                          │
│     - If offline: TODO: Send push notification            │
└───────────────────┬───────────────────────────────────────┘
                    │
                    ▼
         ┌────────────────────────┐
         │  QUEUE INFRASTRUCTURE  │
         │  (user.event.delivery) │
         │   with header filters  │
         └──────────┬──────────────┘
                    │
                    ├──────────────┐ ┌──────────────┐
                    ▼              ▼ ▼              ▼
            ┌─────────────┐    ┌─────────────┐  ┌─────────────┐
            │  Gateway 1  │    │  Gateway 2  │  │  Gateway N  │
            │             │    │             │  │             │
            │ Device A    │    │ Device B    │  │ Device C    │
            │ (filtered)  │    │ (filtered)  │  │ (filtered)  │
            └─────┬───────┘    └─────┬───────┘  └─────┬───────┘
                  │                   │                 │
                  │ ServerEvent       │ ServerEvent     │ ServerEvent
                  ▼                   ▼                 ▼
            ┌─────────────┐    ┌─────────────┐  ┌─────────────┐
            │  Device A   │    │  Device B   │  │  Device C   │
            │  (Mobile)   │    │  (Web)      │  │  (Desktop)  │
            └─────────────┘    └─────────────┘  └─────────────┘
```

## Key Components

### Default Service

#### 1. MessageBusiness (`apps/default/service/business/message_business.go`)
- **SendEvents()**: Entry point for sending messages
- Validates sender has room access
- Saves event to database
- Emits internal event for async processing

#### 2. RoomOutboxLoggingQueue (`apps/default/service/events/outbox_logging.go`)
- Event handler for "room.outbox.logging.event"
- Creates outbox entries for all room subscribers
- Batch inserts to optimize database writes
- Emits EventBroadcast for delivery processing

#### 3. OutboxDeliveryEventHandler (`apps/default/service/events/outbox_delivery.go`)
- Event handler for "outbox.delivery.event"
- Retrieves full event data with content
- Creates EventDelivery proto for each recipient
- Publishes to queue for gateway consumption
- **Lazy initializes publisher** on first use via `service.GetPublisher()`

#### 4. EventDeliveryQueueHandler (`apps/default/service/queues/last_mile_delivery.go`)
- Queue subscriber for "user.event.delivery"
- Queries device service for recipient's devices
- For each device:
  - Publishes to queue **with header filters** (profile_id, device_id)
  - Headers enable queue-level routing to specific devices
- TODO: Implement offline detection and push notifications

### Gateway Service

#### 1. GatewayServer.Connect() (`apps/gateway/service/handlers/gateway.go`)
- **Each device connection creates its own queue subscription**
- Sets header filters: `profile_id={user}` and `device_id={device}`
- **Pull-based model**: Connection actively pulls from queue
- Two goroutines:
  1. **Queue → Device**: Pulls messages from filtered subscription, converts to ServerEvent, sends to device
  2. **Device → Gateway**: Receives heartbeats, acks, and other device requests

#### 2. ConnectionManager (`apps/gateway/service/business/connection.go`)
- **Register()**: Registers connection metadata in Redis cache
- Enables cross-gateway coordination
- Tracks active connections for monitoring

### Queue Architecture

```
┌──────────────────────────────────────────────────────┐
│            user.event.delivery Queue                  │
│                                                        │
│  Messages published with headers:                     │
│    - profile_id: user123                              │
│    - device_id: device456                             │
│                                                        │
│  Subscribers filter by headers:                       │
│    Gateway1: profile_id=user123 AND device_id=device1 │
│    Gateway2: profile_id=user123 AND device_id=device2 │
│    Gateway3: profile_id=user456 AND device_id=device3 │
│                                                        │
│  ✓ Each device stream only receives its own messages │
│  ✓ Multiple gateways can handle different devices    │
│  ✓ Horizontal scaling without coordination overhead  │
└──────────────────────────────────────────────────────┘
```

## Scalability Features

### 1. **Pull-Based Subscriptions**
- Each device stream creates its own filtered queue subscription
- No need to push to connections - they pull their own messages
- Streams naturally backpressure based on their consumption rate

### 2. **Header-Based Routing**
- Queue infrastructure routes messages using headers
- profile_id + device_id ensures messages reach correct device
- No application-level routing logic needed

### 3. **Horizontal Scaling**
- Gateway instances don't need to coordinate
- Each handles its own set of device connections
- Redis cache tracks connection metadata for monitoring
- New gateway instances can be added without coordination

### 4. **Asynchronous Processing**
- Message sending returns immediately after database write
- Outbox processing happens asynchronously via events
- Queue delivery decoupled from message sending

### 5. **Backpressure Handling**
- Slow devices only affect their own stream
- Queue buffers messages per device
- Fast devices aren't slowed by slow ones

## Message Proto Definitions

### EventDelivery (`proto/events/v1/payloads.proto`)
```protobuf
message EventDelivery {
  EventLink event = 1;           // The chat event
  EventReceipt target = 2;     // Recipient info
  google.protobuf.Struct payload = 3;  // Message content
  bool is_compressed = 4;
  int32 retry_count = 5;
}
```

### EventBroadcast
```protobuf
message EventBroadcast {
  EventLink event = 1;              // The chat event
  repeated EventReceipt targets = 2;  // All recipients
  int32 priority = 3;
}
```

## Configuration

### Default Service (`apps/default/config/config.go`)
```go
QueueUserEventDeliveryName string `envDefault:"user.event.delivery"`
QueueUserEventDeliveryURI  string `envDefault:"mem://user.event.delivery"`
```

### Gateway Service (`apps/gateway/config/config.go`)
```go
QueueUserEventDeliveryName string `envDefault:"user.event.delivery"`
QueueUserEventDeliveryURI  string `envDefault:"mem://user.event.delivery"`
CacheURI  string `envDefault:"redis://localhost:6379"`
```

## Future Enhancements

1. **Device Online Detection**: Query gateway cache to check if device is connected before publishing
2. **Push Notifications**: Integrate notification service for offline devices
3. **Message Acknowledgments**: Track delivery and read receipts
4. **Priority Queues**: Support high-priority messages (e.g., @mentions)
5. **Message Compression**: Compress large message payloads
6. **Retry Logic**: Automatic retry for failed deliveries
7. **Dead Letter Queue**: Handle permanently failed messages
8. **Metrics & Monitoring**: Track queue depths, delivery latency, connection counts

## Testing

### Local Development
Both services can use in-memory queues (`mem://`) for development:
```bash
# Default service
QUEUE_USER_EVENT_DELIVERY_URI=mem://user.event.delivery

# Gateway service  
QUEUE_USER_EVENT_DELIVERY_URI=mem://user.event.delivery
```

### Production
Use a distributed queue system (Redis, RabbitMQ, Kafka, etc.):
```bash
# Both services point to same queue infrastructure
QUEUE_USER_EVENT_DELIVERY_URI=redis://redis:6379/delivery
```

## Summary

This architecture achieves:
- ✅ **Real-time delivery** via bidirectional streams
- ✅ **Horizontal scalability** through pull-based subscriptions  
- ✅ **Device isolation** via header-filtered queues
- ✅ **Backpressure handling** per device
- ✅ **Decoupled services** via queue infrastructure
- ✅ **Reliable delivery** with database-backed outbox pattern
- ✅ **Multi-device support** for each user
- ✅ **Gateway independence** - no cross-gateway coordination needed
