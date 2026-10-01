// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Infrastructure Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package infrastructure

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ─── 1. Core Interceptors & Filters ──────────────────────────────────────────

// StandardAPIResponse wraps all API payloads uniformly.
type StandardAPIResponse struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// TransformInterceptor wraps successful HTTP JSON responses into a consistent envelope.
func TransformInterceptor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &interceptingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
	})
}

// LoggingInterceptor logs every HTTP request with path, method, status, and duration.
func LoggingInterceptor(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &interceptingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			logger.Info("HTTP Request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// ExceptionsFilter catches panics and formats uniform 500 error responses without leaking stack traces.
func ExceptionsFilter(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("Unhandled HTTP panic recovered",
						"panic", fmt.Sprintf("%v", rec),
						"path", r.URL.Path,
						"method", r.Method,
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(StandardAPIResponse{
						Success:   false,
						Error:     "Internal server error",
						Timestamp: time.Now().UTC().Format(time.RFC3339),
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type interceptingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *interceptingResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// ─── 2. Auth Guard ───────────────────────────────────────────────────────────

// AuthContextKey defines the context key for authenticated claims.
type AuthContextKey struct{}

// AuthenticatedClaims represents verified JWT or API token claims.
type AuthenticatedClaims struct {
	UserID         string   `json:"user_id"`
	Email          string   `json:"email"`
	OrganizationID string   `json:"organization_id"`
	Roles          []string `json:"roles"`
}

// AuthGuard verifies presence of authorization headers.
type AuthGuard struct {
	jwtSecret string
	logger    *slog.Logger
}

// NewAuthGuard initializes an authentication guard.
func NewAuthGuard(jwtSecret string, logger *slog.Logger) *AuthGuard {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthGuard{
		jwtSecret: jwtSecret,
		logger:    logger.With("component", "auth_guard"),
	}
}

// Middleware is intentionally not provided.
//
// SECURITY: this guard was removed because it was a booby trap. It checked only
// that an Authorization header was present and non-empty, never validated the
// token, then injected a hardcoded claims object with UserID
// "authenticated-user". Wiring it anywhere would have granted a valid session
// to anyone sending `Authorization: Bearer x`
// (AUDIT_REMEDIATION.md F-14).
//
// Use one of the real guards instead:
//   - internal/auth.Authenticator.Middleware   — the production session guard
//   - internal/mcp/manager/api/middleware     — the MCP transport guard
//
// NewAuthGuard and AuthGuard are retained only so existing call sites fail to
// compile loudly rather than silently degrading.

// ─── 3. Pool Error Handler ───────────────────────────────────────────────────

// PoolErrorHandlerService intercepts database connection pooling errors and coordinates retries or alerts.
type PoolErrorHandlerService struct {
	logger *slog.Logger
}

// NewPoolErrorHandlerService constructs the handler service.
func NewPoolErrorHandlerService(logger *slog.Logger) *PoolErrorHandlerService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PoolErrorHandlerService{
		logger: logger.With("component", "pool_error_handler"),
	}
}

// HandlePoolError logs and classifies database connection pool errors.
func (h *PoolErrorHandlerService) HandlePoolError(err error, operation string) {
	if err == nil {
		return
	}
	h.logger.Error("Database connection pool error encountered",
		"operation", operation,
		"error", err.Error(),
	)
}
