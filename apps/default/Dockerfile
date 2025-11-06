# ---------- Builder ----------
FROM golang:1.25 AS builder

WORKDIR /app

# Inject version info from build args
ARG VERSION=dev
ARG REVISION=none
ARG BUILDTIME
ARG TARGETOS
ARG TARGETARCH

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the actual source
COPY ./apps/default ./apps/default
COPY ./proto ./proto

# Build static binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w \
        -X main.version=${VERSION} \
        -X main.commit=${REVISION} \
        -X main.date=${BUILDTIME}" \
    -o /drone ./apps/default/cmd/main.go

# ---------- Final ----------
FROM cgr.dev/chainguard/static:latest

# Copy binary from builder
COPY --from=builder /drone /drone

# Use non-root user
USER nonroot:nonroot

# Expose your app ports
EXPOSE 80 50051

# Add OCI metadata labels
ARG VERSION
ARG REVISION
ARG BUILDTIME
LABEL org.opencontainers.image.title="Drone Service"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.revision=$REVISION
LABEL org.opencontainers.image.created=$BUILDTIME
LABEL org.opencontainers.image.source="https://github.com/antinvestor/service-chat"

# Set entrypoint
ENTRYPOINT ["/drone"]
