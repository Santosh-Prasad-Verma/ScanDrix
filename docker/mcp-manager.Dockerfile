# ═══════════════════════════════════════════════════════════════
# ScanDrix AI - Enterprise Code Review Platform
# Copyright (c) 2026 ScanDrix AI. All rights reserved.
# ═══════════════════════════════════════════════════════════════

FROM golang:alpine AS builder
ENV GOTOOLCHAIN=auto
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /build/bin/scandrix-mcp-manager ./cmd/mcp-manager

FROM alpine:3.21 AS runner
RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -g 10001 -S scandrix && \
    adduser -u 10001 -S scandrix -G scandrix -h /home/scandrix
WORKDIR /app
COPY --from=builder /build/bin/scandrix-mcp-manager /app/bin/scandrix-mcp-manager
RUN chown -R scandrix:scandrix /app
USER scandrix:scandrix
EXPOSE 3101
HEALTHCHECK --interval=30s --timeout=10s --retries=3 --start-period=10s \
    CMD curl -f http://127.0.0.1:3101/health || exit 1
ENTRYPOINT ["/app/bin/scandrix-mcp-manager"]
