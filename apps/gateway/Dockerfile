ARG TARGETOS=linux
ARG TARGETARCH=amd64

# ---------- Builder ----------
FROM golang:1.25 AS builder

WORKDIR /app

# Inject version info from build args
ARG VERSION=dev
ARG REVISION=none
ARG BUILDTIME

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the actual source
COPY ./apps/gateway ./apps/gateway
COPY ./proto ./proto

# Build static binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
     -ldflags="-s -w \
         -X github.com/pitabwire/frame.Version=${VERSION} \
         -X github.com/pitabwire/frame.Commit=${REVISION} \
         -X github.com/pitabwire/frame.Date=${BUILDTIME}" \
    -o /gateway ./apps/gateway/cmd/main.go

# ---------- Final ----------
FROM cgr.dev/chainguard/static:latest

# Copy binary from builder
COPY --from=builder /gateway /gateway

# Use non-root user
USER 65532:65532

# Expose your app ports
EXPOSE 80

# Add OCI metadata labels
ARG VERSION
ARG REVISION
ARG BUILDTIME
LABEL org.opencontainers.image.title="Gateway Service"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.revision=$REVISION
LABEL org.opencontainers.image.created=$BUILDTIME
LABEL org.opencontainers.image.source="https://github.com/antinvestor/service-chat"

# Set entrypoint
ENTRYPOINT ["/gateway"]
