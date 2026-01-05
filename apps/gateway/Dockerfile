ARG TARGETOS=linux
ARG TARGETARCH=amd64

# ---------- Builder ----------
FROM golang:1.25 AS builder

WORKDIR /app

# Inject version info from build args
ARG REPOSITORY
ARG VERSION=dev
ARG REVISION=none
ARG BUILDTIME

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the actual source
COPY ./internal ./internal
COPY ./apps/gateway ./apps/gateway

# Build static binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
     -ldflags="-s -w \
         -X github.com/pitabwire/frame/version.Repository=${REPOSITORY} \
         -X github.com/pitabwire/frame/version.Version=${VERSION} \
         -X github.com/pitabwire/frame/version.Commit=${REVISION} \
         -X github.com/pitabwire/frame/version.Date=${BUILDTIME}" \
    -o /gateway ./apps/gateway/cmd/main.go

# ---------- Final ----------
FROM cgr.dev/chainguard/static:latest
# Add Maintainer Info
LABEL maintainer="Bwire Peter <bwire517@gmail.com>"

# Use non-root user
USER 65532:65532

# Expose your app ports
EXPOSE 80

# Add OCI metadata labels
ARG REPOSITORY
ARG VERSION
ARG REVISION
ARG BUILDTIME
LABEL org.opencontainers.image.title="Gateway Service"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.revision=$REVISION
LABEL org.opencontainers.image.created=$BUILDTIME
LABEL org.opencontainers.image.source=$REPOSITORY

WORKDIR /
# Copy binary from builder
COPY --from=builder /gateway /gateway

# Set entrypoint
ENTRYPOINT ["/gateway"]
