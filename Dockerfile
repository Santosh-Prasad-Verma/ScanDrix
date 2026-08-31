# ==============================================================================
# ScanDrix Enterprise Production Multi-Stage Dockerfile
# Security Hardened: Non-Root Execution, Distroless/Static Alpine Base, CGO=0
# ==============================================================================

# Stage 1: Build Binaries
FROM golang:alpine AS builder
ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree
COPY . .

# Build statically linked, stripped binaries
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
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

RUN apk add --no-cache ca-certificates tzdata git && \
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

ENTRYPOINT ["/app/bin/scandrix-server"]
