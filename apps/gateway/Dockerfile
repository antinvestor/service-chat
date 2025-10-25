FROM golang:1.25 AS builder

# Set working directory
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Copy project files
COPY ./apps/gateway ./apps/gateway
COPY ./internal ./internal

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags '-extldflags "-static"' -o binary ./apps/gateway/cmd/main.go

# Final minimal image
FROM gcr.io/distroless/static:nonroot

# Copy from builder stage
COPY --from=builder /app/binary /gateway

# Run as non-root user
USER 65532:65532

# Document ports
EXPOSE 80
EXPOSE 50051

# Set working directory
WORKDIR /

# Run the binary
ENTRYPOINT ["/gateway"]
