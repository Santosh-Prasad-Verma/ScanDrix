# Stage 1: Build Binaries
# Pinned to the Go release go.mod requires (1.25.3). An unpinned
# `golang:alpine` drifts with the tag, so builds either break or silently
# download a toolchain at image-build time (AUDIT_REMEDIATION.md F-58).
FROM golang:1.25-alpine AS builder
ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build statically linked, stripped binaries with multi-arch support
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ENV CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}
RUN go build -ldflags="-s -w" -o /build/bin/scandrix-api ./cmd/api && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-server ./cmd/server && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-webhooks ./cmd/webhooks && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-worker ./cmd/worker && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-migrate ./cmd/migrate && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-cli ./cmd/cli && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-mcp-manager ./cmd/mcp-manager && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-ast-cli ./cmd/ast-cli && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-analytics-cli ./cmd/analytics-cli && \
    go build -ldflags="-s -w" -o /build/bin/scandrix-try ./cmd/try

# Stage 2: Minimal Production Image
FROM alpine:3.21 AS runner

RUN apk add --no-cache ca-certificates tzdata git wget && \
    addgroup -g 10001 -S scandrix && \
    adduser -u 10001 -S scandrix -G scandrix -h /home/scandrix

WORKDIR /app

# Copy compiled binaries and migrations from builder
COPY --from=builder /build/bin/ /app/bin/
COPY --from=builder /build/migrations/ /app/migrations/

# Create runtime directories with non-root ownership
RUN mkdir -p /app/data /app/sandboxes /app/logs && \
    chown -R scandrix:scandrix /app

USER scandrix:scandrix

EXPOSE 8080 8081 9090

# This image is shared by api, webhooks, worker and migrate. The health probe
# port differs per service: api/webhooks serve on 8080, the worker on
# WORKER_HEALTH_PORT (8082). Hardcoding 8080 left the worker permanently
# unhealthy. The shell expands this from the container environment, so the
# port is resolved at run time rather than baked in.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- "http://localhost:${HEALTH_PROBE_PORT:-8080}/healthz" || exit 1

ENTRYPOINT ["/app/bin/scandrix-server"]
