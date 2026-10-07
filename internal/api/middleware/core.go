package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// ExceptionsFilter provides centralized panic recovery and error formatting (Master Rule 5.7).
// It ensures stack traces, database errors, and system internals are never returned to clients.
func ExceptionsFilter(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID := middleware.GetReqID(r.Context())
					stack := string(debug.Stack())

					// Server-side audit logging with full stack trace
					logger.Error("Unhandled panic intercepted in HTTP pipeline",
						"error", fmt.Sprintf("%v", rec),
						"request_id", reqID,
						"path", r.URL.Path,
						"method", r.Method,
						"stack", stack,
					)

					// Sanitized client response — zero internal leaks (Master Rule 5.7)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"success":    false,
						"error":      "An internal server error occurred",
						"request_id": reqID,
						"timestamp":  time.Now().UTC().Format(time.RFC3339),
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func (rw *statusResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (rw *statusResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// LoggingInterceptor logs structured HTTP request and response metrics.
func LoggingInterceptor(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := middleware.GetReqID(r.Context())
			clientIP := ExtractClientIP(r)

			sw := &statusResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(sw, r)

			duration := time.Since(start)

			// Skip detailed logging for high-frequency health probes
			path := r.URL.Path
			if path == "/healthz" || path == "/livez" || path == "/metrics" {
				return
			}

			level := slog.LevelInfo
			if sw.statusCode >= 500 {
				level = slog.LevelError
			} else if sw.statusCode >= 400 {
				level = slog.LevelWarn
			}

			logger.Log(r.Context(), level, "HTTP request processed",
				"method", r.Method,
				"path", path,
				"status", sw.statusCode,
				"duration_ms", duration.Milliseconds(),
				"bytes", sw.bytesWritten,
				"client_ip", clientIP,
				"request_id", reqID,
			)
		})
	}
}

// ResponseEnvelope standardizes API responses into a unified JSON format.
type ResponseEnvelope struct {
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Meta      any    `json:"meta,omitempty"`
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// RespondJSON sends a standardized JSON response matching TransformInterceptor specifications.
func RespondJSON(w http.ResponseWriter, r *http.Request, statusCode int, data any, meta any) {
	reqID := middleware.GetReqID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	envelope := ResponseEnvelope{
		Success:   statusCode >= 200 && statusCode < 300,
		Data:      data,
		Meta:      meta,
		RequestID: reqID,
	}

	_ = json.NewEncoder(w).Encode(envelope)
}

// RespondError sends a standardized error JSON response.
func RespondError(w http.ResponseWriter, r *http.Request, statusCode int, userMessage string) {
	reqID := middleware.GetReqID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	envelope := ResponseEnvelope{
		Success:   false,
		Error:     userMessage,
		RequestID: reqID,
	}

	_ = json.NewEncoder(w).Encode(envelope)
}
