# Chat Gateway Service

The Chat Gateway Service is a lightweight, horizontally scalable service responsible for maintaining real-time WebSocket/HTTP2 connections with edge devices. It acts as a proxy between edge devices and the core chat service, enabling efficient real-time message delivery.

## Architecture

```
Edge Devices <---> Gateway Service <---> Core Chat Service
```

### Key Responsibilities

1. **Connection Management**: Maintains persistent bidirectional connections with edge devices
2. **Message Broadcasting**: Efficiently broadcasts room events to connected devices
3. **Request Forwarding**: Forwards API requests from devices to the core chat service
4. **Scalability**: Designed to be horizontally scalable to handle millions of concurrent connections

## Features

- **Real-time Communication**: WebSocket/HTTP2 bidirectional streaming
- **Connection Tracking**: Per-device and per-room connection management
- **Heartbeat Monitoring**: Automatic detection and cleanup of stale connections
- **Room Subscriptions**: Dynamic subscription management for rooms
- **Authentication**: JWT-based authentication for all connections
- **Telemetry**: OpenTelemetry integration for monitoring

## Configuration

Configuration is handled through environment variables:

```bash
# Chat Service Connection
CHAT_SERVICE_URI=127.0.0.1:7010

# Connection Management
MAX_CONNECTIONS_PER_DEVICE=1
CONNECTION_TIMEOUT_SEC=300
HEARTBEAT_INTERVAL_SEC=30

# Rate Limiting
MAX_EVENTS_PER_SECOND=100

# Cache Configuration (Required for horizontal scaling)
# Connection metadata is stored in cache to coordinate across gateway instances
CACHE_URI=redis://localhost:6379

# OAuth2/JWT Settings (inherited from frame.ConfigurationDefault)
OAUTH2_ISSUER=https://auth.example.com
OAUTH2_JWT_VERIFY_AUDIENCE=service_chat_gateway
OAUTH2_TOKEN_ENDPOINT=https://auth.example.com/token
JWT_CLIENT_ID=gateway_client
JWT_CLIENT_SECRET=secret

# Server Ports
HTTP_PORT=80
GRPC_PORT=50051
```

## Deployment

### Docker

Build the Docker image:
```bash
docker build -f apps/gateway/Dockerfile -t chat-gateway:latest .
```

Run the container:
```bash
docker run -p 80:80 -p 50051:50051 \
  -e CHAT_SERVICE_URI=chat-service:7010 \
  -e CACHE_URI=redis://redis:6379 \
  -e OAUTH2_ISSUER=https://auth.example.com \
  chat-gateway:latest
```

### Docker Compose

Example with Redis:
```yaml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data

  chat-gateway:
    image: chat-gateway:latest
    ports:
      - "80:80"
      - "50051:50051"
    environment:
      CHAT_SERVICE_URI: chat-service:7010
      CACHE_URI: redis://redis:6379
      OAUTH2_ISSUER: https://auth.example.com
    depends_on:
      - redis

volumes:
  redis-data:
```

### Kubernetes

The gateway service is designed to be deployed as a Kubernetes Deployment with:
- Horizontal Pod Autoscaler (HPA) based on connection count
- Service for load balancing
- Ingress for external access

Example deployment configuration:
```yaml
# Redis for connection metadata
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
---
apiVersion: v1
kind: Service
metadata:
  name: redis
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
---
# Chat Gateway
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chat-gateway
spec:
  replicas: 3
  selector:
    matchLabels:
      app: chat-gateway
  template:
    metadata:
      labels:
        app: chat-gateway
    spec:
      containers:
      - name: gateway
        image: chat-gateway:latest
        ports:
        - containerPort: 80
        - containerPort: 50051
        env:
        - name: CHAT_SERVICE_URI
          value: "chat-service:7010"
        - name: CACHE_URI
          value: "redis://redis:6379"
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

## API

The gateway implements the `ChatService` gRPC/Connect API with a focus on:

### Primary Method
- `Connect(stream StreamRequest) returns (stream ServerEvent)`: Bidirectional streaming for real-time communication

### Forwarded Methods
All other methods are forwarded to the core chat service:
- `SendEvent`
- `GetHistory`
- `CreateRoom`
- `SearchRooms`
- `UpdateRoom`
- `DeleteRoom`
- `AddRoomSubscriptions`
- `RemoveRoomSubscriptions`
- `UpdateSubscriptionRole`
- `SearchRoomSubscriptions`

## Monitoring

### Metrics
- Active connection count
- Connection duration
- Message throughput
- Error rates

### Health Checks
The service provides health check endpoints:
- `/healthz`: Liveness probe
- `/readyz`: Readiness probe

## Development

### Running Locally

```bash
# Start Redis (using Docker)
docker run -d -p 6379:6379 redis:7-alpine

# Set required environment variables
export CHAT_SERVICE_URI=127.0.0.1:7010
export CACHE_URI=redis://localhost:6379
export OAUTH2_ISSUER=https://auth.example.com

# Run the gateway
go run apps/gateway/cmd/main.go
```

### Testing

```bash
# Run unit tests
go test ./apps/gateway/...

# Run integration tests
go test ./apps/gateway/... -tags=integration
```

## Architecture Decisions

### Why a Separate Gateway?

1. **Scalability**: The gateway can scale independently based on connection count
2. **Resource Isolation**: Connection management is resource-intensive and isolated from business logic
3. **Deployment Flexibility**: Different scaling strategies for connections vs. business logic
4. **Security**: Additional layer for rate limiting and DDoS protection

### Connection Management

The gateway uses a hybrid storage approach for managing connections:

**Local Storage (in-memory)**:
- Active WebSocket/HTTP2 streams (cannot be serialized)
- Immediate access for message delivery

**Cache Storage (Redis)**:
- Connection metadata (profile ID, device ID, room subscriptions, timestamps)
- Room-to-connection mappings for efficient broadcasting
- Shared across all gateway instances for coordination

This design enables:
- **Horizontal Scaling**: Multiple gateway instances can coordinate through the shared cache
- **Connection Discovery**: Any gateway can query which devices are connected to which rooms
- **State Persistence**: Connection metadata survives gateway restarts (though active streams don't)
- **Efficient Broadcasting**: Room-based indices allow quick lookup of relevant connections

Key Features:
- Each device maintains a single connection per gateway instance
- Connections are tracked per-room for efficient broadcasting
- Stale connections are automatically cleaned up based on heartbeat intervals
- Gateway instances are identified by unique IDs for debugging and monitoring

**Cache Data Structure**:

```
# Connection metadata
gateway:conn:{profileID}:{deviceID} -> JSON {
  "profile_id": "string",
  "device_id": "string", 
  "room_ids": ["room1", "room2"],
  "last_active": 1234567890,
  "last_heartbeat": 1234567890,
  "connected": 1234567890,
  "gateway_id": "gateway-1234567890"
}

# Room-to-connection mappings
gateway:room:{roomID} -> JSON ["profileID:deviceID", ...]
```

All cache entries have TTL of 2x connection timeout to automatically clean up stale data.

### Load Balancing

The gateway uses sticky sessions (session affinity) to ensure:
- A device reconnects to the same gateway instance when possible
- Efficient room subscription management
- Reduced connection churn

## Troubleshooting

### Common Issues

1. **Connections timing out**: Check `CONNECTION_TIMEOUT_SEC` setting
2. **High memory usage**: Reduce `MAX_CONNECTIONS_PER_DEVICE` or scale horizontally
3. **Slow message delivery**: Check network latency to chat service

### Logs

Enable debug logging:
```bash
export LOG_LEVEL=debug
```

## Future Enhancements

- [x] Redis cache for connection metadata (implemented)
- [ ] Redis pub/sub for cross-gateway message broadcasting
- [ ] Connection state persistence for graceful failover
- [ ] Advanced rate limiting per user/room
- [ ] WebRTC signaling support
- [ ] Message queuing for offline devices
- [ ] Metrics export for Prometheus
- [ ] Circuit breaker for chat service calls
