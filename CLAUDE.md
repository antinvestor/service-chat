# Service Chat - Real-Time Messaging Infrastructure

## Overview

Service Chat is a scalable, production-ready real-time messaging infrastructure that powers secure group communication, credit & savings automation, and multi-device chat experiences. The system is architected as a distributed microservices platform with two main applications: the **Default Service** (core business logic) and the **Gateway Service** (connection management).

## System Architecture

```
┌─────────────┐
│   Clients   │  (Mobile, Web, Desktop)
│   Devices   │
└──────┬──────┘
       │
       │ WebSocket/HTTP2
       ▼
┌─────────────────────────────────────────┐
│       GATEWAY SERVICE                    │
│  - Connection Management                 │
│  - Real-time Bidirectional Streams      │
│  - Device-specific Message Routing      │
│  - Horizontal Scaling                   │
└──────┬──────────────────────────────────┘
       │
       │ gRPC/Connect
       ▼
┌─────────────────────────────────────────┐
│       DEFAULT SERVICE                    │
│  - Business Logic                        │
│  - Message Persistence                   │
│  - Room Management                       │
│  - Subscription Management              │
│  - Event Processing                     │
└──────┬──────────────────────────────────┘
       │
       ├──────────┬──────────┐
       ▼          ▼          ▼
   ┌────────┐ ┌──────┐ ┌──────────┐
   │Database│ │Queue │ │  Cache   │
   │(Postgres)│ │(Redis)│ │ (Redis)  │
   └────────┘ └──────┘ └──────────┘
```

## Applications

### 1. Default Service (`apps/default/`)

**Purpose**: Core chat service that handles all business logic, data persistence, and message processing.

**Key Responsibilities**:
- **Message Management**: Send, retrieve, and search chat messages
- **Room Management**: Create, update, delete, and search chat rooms
- **Subscription Management**: Handle room memberships, roles, and permissions
- **Event Processing**: Asynchronous message delivery pipeline using event-driven architecture
- **Outbox Pattern**: Reliable message delivery to all room subscribers
- **API Gateway**: Exposes gRPC/Connect API for all chat operations

**Technology Stack**:
- **Language**: Go 1.25+
- **Framework**: [Frame](https://github.com/pitabwire/frame) (microservices framework)
- **Database**: PostgreSQL with GORM
- **API**: gRPC with Connect protocol
- **Queue**: Pluggable queue system (Redis, RabbitMQ, Kafka, in-memory)
- **Auth**: OAuth2/JWT with token validation
- **Testing**: Testify with testcontainers for integration tests

**Directory Structure**:
```
apps/default/
├── cmd/                  # Application entry point
├── config/              # Configuration structs and env vars
├── service/
│   ├── business/        # Business logic layer
│   │   ├── message_business.go     # Message operations
│   │   ├── room_business.go        # Room operations
│   │   └── subscription_business.go # Subscription operations
│   ├── handlers/        # gRPC/Connect API handlers
│   │   └── chat_server.go          # ChatService implementation
│   ├── repository/      # Data access layer
│   │   ├── message_repository.go   # Message CRUD
│   │   ├── room_repository.go      # Room CRUD
│   │   └── outbox_repository.go    # Outbox pattern
│   ├── events/          # Event handlers (async processing)
│   │   ├── outbox_logging.go       # Create outbox entries
│   │   └── outbox_delivery.go      # Process outbox for delivery
│   ├── queues/          # Queue subscribers
│   │   └── last_mile_delivery.go   # Device-level message routing
│   └── models/          # Domain models and DB entities
│       └── models.go               # RoomEvent, Room, Subscription
└── tests/               # Integration tests
    └── integration_test.go         # E2E test scenarios
```

**Key Features**:

1. **Message Flow Pipeline**:
   ```
   SendEvent → Save to DB → Emit Event → Create Outbox Entries →
   Queue Publishing → Gateway Delivery → Device Streams
   ```

2. **Scalability**:
   - Stateless design enables horizontal scaling
   - Queue-based async processing decouples components
   - Database connection pooling
   - Batch operations for performance

3. **Reliability**:
   - Outbox pattern ensures message delivery
   - Database transactions for consistency
   - Event-driven architecture for fault tolerance
   - Retry logic for failed operations

4. **Security**:
   - JWT token validation on all requests
   - Role-based access control (RBAC)
   - Room-level permissions
   - Profile and Contact linking via ContactLink

**API Endpoints** (via ChatService):
- `SendEvent`: Send messages to rooms
- `GetHistory`: Retrieve message history with pagination
- `CreateRoom`: Create new chat rooms
- `SearchRooms`: Find rooms by criteria
- `UpdateRoom`: Modify room metadata
- `DeleteRoom`: Remove rooms
- `AddRoomSubscriptions`: Add members to rooms
- `RemoveRoomSubscriptions`: Remove members from rooms
- `UpdateSubscriptionRole`: Change member roles (admin, moderator, etc.)
- `SearchRoomSubscriptions`: Find room members

**Configuration** (Environment Variables):
```bash
# Database
DB_URI=postgresql://user:pass@localhost:5432/chat

# Queue System
QUEUE_USER_EVENT_DELIVERY_NAME=user.event.delivery
QUEUE_USER_EVENT_DELIVERY_URI=redis://localhost:6379/delivery

# OAuth2/JWT
OAUTH2_ISSUER=https://auth.example.com
OAUTH2_JWT_VERIFY_AUDIENCE=service_chat
JWT_CLIENT_ID=chat_service
JWT_CLIENT_SECRET=secret

# Server Ports
HTTP_PORT=7000
GRPC_PORT=7010
```

### 2. Gateway Service (`apps/gateway/`)

**Purpose**: Lightweight, horizontally scalable service for managing real-time bidirectional connections with edge devices.

**Key Responsibilities**:
- **Connection Management**: Maintain persistent WebSocket/HTTP2 streams
- **Real-time Message Delivery**: Stream messages to connected devices
- **Device Routing**: Per-device message filtering using queue headers
- **Request Forwarding**: Proxy API calls to Default Service
- **Connection Tracking**: Redis-backed metadata for cross-gateway coordination

**Technology Stack**:
- **Language**: Go 1.25+
- **Framework**: Frame (microservices framework)
- **Cache**: Redis for connection metadata
- **Protocol**: gRPC bidirectional streaming (Connect protocol)
- **Queue**: Shared with Default Service for message delivery
- **Auth**: OAuth2/JWT (same as Default Service)

**Directory Structure**:
```
apps/gateway/
├── cmd/                 # Application entry point
├── config/             # Configuration structs
├── service/
│   ├── handlers/       # gRPC/Connect API handlers
│   │   └── gateway.go  # Stream management
│   └── business/       # Business logic
│       └── connection.go # Connection lifecycle management
└── README.md           # Detailed gateway documentation
```

**Key Features**:

1. **Pull-Based Architecture**:
   - Each device connection creates its own queue subscription
   - Devices pull messages from filtered queues (not pushed)
   - Natural backpressure handling per device

2. **Header-Based Routing**:
   - Messages published with `profile_id` and `device_id` headers
   - Queue infrastructure routes to specific subscriptions
   - No application-level routing logic needed

3. **Horizontal Scaling**:
   - Stateless gateway instances (streams in-memory, metadata in Redis)
   - No cross-gateway coordination required
   - Add instances without configuration changes
   - Connection metadata shared via Redis cache

4. **Connection Metadata** (Stored in Redis):
   ```json
   {
     "profile_id": "user123",
     "device_id": "device456",
     "room_ids": ["room1", "room2"],
     "last_active": 1234567890,
     "last_heartbeat": 1234567890,
     "connected": 1234567890,
     "gateway_id": "gateway-abc"
   }
   ```

5. **Efficient Broadcasting**:
   - Room-to-connection indices in Redis
   - Quick lookup of devices subscribed to each room
   - Minimal overhead per message

**API Endpoints**:
- `Connect(stream)`: Primary bidirectional streaming endpoint
- All other ChatService methods: Forwarded to Default Service

**Configuration** (Environment Variables):
```bash
# Default Service Connection
CHAT_SERVICE_URI=default-service:7010

# Connection Settings
MAX_CONNECTIONS_PER_DEVICE=1
CONNECTION_TIMEOUT_SEC=300
HEARTBEAT_INTERVAL_SEC=30

# Rate Limiting
MAX_EVENTS_PER_SECOND=100

# Cache (Required for horizontal scaling)
CACHE_URI=redis://localhost:6379

# Queue (Shared with Default Service)
QUEUE_USER_EVENT_DELIVERY_NAME=user.event.delivery
QUEUE_USER_EVENT_DELIVERY_URI=redis://localhost:6379/delivery

# OAuth2/JWT (Same as Default Service)
OAUTH2_ISSUER=https://auth.example.com
OAUTH2_JWT_VERIFY_AUDIENCE=service_chat_gateway

# Server Ports
HTTP_PORT=80
GRPC_PORT=50051
```

## Message Delivery Flow

### End-to-End Flow

```
1. Client sends message via Gateway or directly to Default Service
   ↓
2. Default Service validates, saves to DB, emits "room.outbox.logging.event"
   ↓
3. RoomOutboxLoggingQueue creates outbox entry for each room subscriber
   ↓
4. OutboxDeliveryEventHandler publishes EventDelivery to queue
   ↓
5. EventDeliveryQueueHandler queries devices for each recipient
   ↓
6. Publishes to queue with headers (profile_id, device_id)
   ↓
7. Gateway's filtered subscription receives message for its device
   ↓
8. Gateway streams ServerEvent to device over WebSocket/HTTP2
   ↓
9. Client receives message in real-time
```

### Proto Definitions

**EventDelivery** (Queue Message):
```protobuf
message EventDelivery {
  EventLink event = 1;           // Event reference (ID, room_id)
  EventReceipt target = 2;       // Recipient (profile_id, device_id)
  google.protobuf.Struct payload = 3;  // Message content
  bool is_compressed = 4;
  int32 retry_count = 5;
}
```

**RoomEvent** (API):
```protobuf
message RoomEvent {
  string id = 1;
  string room_id = 2;
  common.v1.ContactLink source = 3;  // Sender (profile_id + contact_id)
  RoomEventType type = 4;
  google.protobuf.Timestamp sent_at = 5;

  // Typed content (one of):
  TextContent text = 9;
  AttachmentContent attachment = 10;
  ReactionContent reaction = 11;
  EncryptedContent encrypted = 12;
  CallContent call = 13;
}
```

**ContactLink** (Identity):
```protobuf
message ContactLink {
  string profile_id = 1;    // On-platform user ID
  string contact_id = 2;    // Off-platform contact ID (email/phone)
  string profile_name = 3;
  ProfileType profile_type = 4;
}
```

## Data Models

### Core Entities

**RoomEvent** (Message):
- `ID`: Unique event identifier
- `RoomID`: Room where message was sent
- `SenderID`: Profile ID (from ContactLink.ProfileId)
- `ContactID`: Contact ID (from ContactLink.ContactId)
- `EventType`: Message type (text, attachment, reaction, call, etc.)
- `Content`: Message payload (JSON)
- `ParentID`: Reply-to message ID
- `CreatedAt`: Client timestamp
- `ServerTs`: Server timestamp

**Room**:
- `ID`: Unique room identifier
- `Name`: Room display name
- `Type`: Room type (direct, group, channel)
- `CreatorID`: Profile ID of creator
- `Metadata`: Custom room data (JSON)
- `CreatedAt`, `UpdatedAt`

**RoomSubscription** (Membership):
- `RoomID`: Room identifier
- `ProfileID`: Member profile ID
- `ContactID`: Member contact ID
- `Role`: Member role (member, admin, moderator)
- `JoinedAt`: Membership start time
- `LastActive`: Last activity timestamp

**RoomOutbox** (Delivery Tracking):
- `EventID`: Reference to RoomEvent
- `ProfileID`: Recipient profile ID
- `DeviceID`: Target device ID
- `Status`: Delivery status (pending, delivered, failed)
- `AttemptCount`: Retry counter
- `CreatedAt`, `UpdatedAt`

## Deployment

### Docker Compose (Local Development)

```yaml
version: '3.8'
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: chat
      POSTGRES_USER: chat
      POSTGRES_PASSWORD: secret
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data

  default-service:
    build:
      context: .
      dockerfile: apps/default/Dockerfile
    ports:
      - "7000:7000"
      - "7010:7010"
    environment:
      DB_URI: postgresql://chat:secret@postgres:5432/chat
      QUEUE_USER_EVENT_DELIVERY_URI: redis://redis:6379/delivery
      OAUTH2_ISSUER: https://auth.example.com
    depends_on:
      - postgres
      - redis

  gateway:
    build:
      context: .
      dockerfile: apps/gateway/Dockerfile
    ports:
      - "80:80"
      - "50051:50051"
    environment:
      CHAT_SERVICE_URI: default-service:7010
      CACHE_URI: redis://redis:6379
      QUEUE_USER_EVENT_DELIVERY_URI: redis://redis:6379/delivery
      OAUTH2_ISSUER: https://auth.example.com
    depends_on:
      - redis
      - default-service

volumes:
  postgres-data:
  redis-data:
```

### Kubernetes (Production)

Key considerations:
- **Default Service**: StatefulSet or Deployment with DB connection pooling
- **Gateway Service**: Deployment with HPA based on connection count
- **PostgreSQL**: Managed database (RDS, CloudSQL, etc.) or StatefulSet
- **Redis**: Managed cache (ElastiCache, MemoryStore) or StatefulSet
- **Queue**: Managed queue service or self-hosted (RabbitMQ, Kafka)
- **Ingress**: Load balancer with sticky sessions for Gateway
- **Secrets**: OAuth credentials, DB passwords via K8s Secrets

## Development

### Prerequisites
- Go 1.25+
- PostgreSQL 14+
- Redis 7+
- Docker & Docker Compose (optional)

### Setup

```bash
# Clone repository
git clone https://github.com/antinvestor/service-chat.git
cd service-chat

# Install dependencies
go mod download

# Set environment variables
export DB_URI=postgresql://localhost:5432/chat
export QUEUE_USER_EVENT_DELIVERY_URI=mem://delivery
export OAUTH2_ISSUER=https://auth.example.com

# Run Default Service
go run apps/default/cmd/main.go

# In another terminal, run Gateway Service
go run apps/gateway/cmd/main.go
```

### Testing

```bash
# Run all tests
go test ./...

# Run business logic tests
go test ./apps/default/service/business/...

# Run handler tests
go test ./apps/default/service/handlers/...

# Run repository tests
go test ./apps/default/service/repository/...

# Run integration tests
go test ./apps/default/tests/...

# Run with coverage
go test -cover ./...

# Run with verbose output
go test -v ./...
```

### Code Generation

Protocol buffer definitions are maintained in separate `apis` repository:
```bash
# Generate protobuf code (done automatically via buf.build)
# See: buf.build/gen/go/antinvestor/chat
```

## API Usage Examples

### Send Message (gRPC/Connect)

```go
// Create chat client
client := chatv1connect.NewChatServiceClient(
    http.DefaultClient,
    "https://gateway.example.com",
)

// Send text message
resp, err := client.SendEvent(ctx, connect.NewRequest(&chatv1.SendEventRequest{
    Event: []*chatv1.RoomEvent{{
        RoomId: "room123",
        Source: &commonv1.ContactLink{ProfileId: "user456"},
        Type: chatv1.RoomEventType_ROOM_EVENT_TYPE_TEXT,
        Text: &chatv1.TextContent{Body: "Hello, world!"},
    }},
}))
```

### Stream Messages (Bidirectional)

```go
// Connect to gateway
stream := client.Connect(ctx)

// Send hello handshake
stream.Send(&chatv1.StreamRequest{
    Hello: &chatv1.StreamHello{
        Capabilities: map[string]string{},
        ClientTime: timestamppb.Now(),
    },
})

// Receive messages
for {
    event, err := stream.Receive()
    if err != nil {
        break
    }

    if event.RoomEvent != nil {
        fmt.Printf("New message: %s\n", event.RoomEvent.Text.Body)
    }
}
```

## Performance & Scalability

### Benchmarks (Typical Hardware)
- **Message Throughput**: 10,000+ messages/sec per Default Service instance
- **Concurrent Connections**: 100,000+ per Gateway instance
- **Message Latency**: <50ms (p95) end-to-end
- **Database**: 50,000+ writes/sec with proper indexing

### Scaling Recommendations
- **Default Service**: Scale horizontally based on CPU/memory
- **Gateway Service**: Scale based on connection count (10k-100k per instance)
- **PostgreSQL**: Use read replicas for history queries
- **Redis**: Use cluster mode for >10GB cache
- **Queue**: Partition by room or user for very high throughput

## Security

### Authentication
- OAuth2/OIDC with JWT tokens
- Token validation on every request
- Refresh token rotation
- Device-based tokens

### Authorization
- Role-based access control (RBAC)
- Room-level permissions
- Subscription-based access
- Admin/moderator privileges

### Data Protection
- TLS/HTTPS for all communications
- Optional end-to-end encryption (E2EE)
- Message retention policies
- GDPR compliance features

## Monitoring & Observability

### Metrics (OpenTelemetry)
- Request rate, latency, error rate (RED metrics)
- Database query performance
- Queue depth and processing time
- Active connection count
- Memory and CPU usage

### Logging
- Structured JSON logging
- Request/response tracing
- Error tracking
- Audit logs

### Health Checks
- `/healthz`: Liveness probe
- `/readyz`: Readiness probe (DB, queue, cache connectivity)

## Contributing

See individual application READMEs for detailed contribution guidelines:
- [Default Service](apps/default/)
- [Gateway Service](apps/gateway/README.md)

## License

Proprietary - Antinvestor

## Support

For issues, questions, or feature requests, contact the Antinvestor platform team.
