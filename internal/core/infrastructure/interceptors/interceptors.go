package interceptors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/log"
)

// StandardDataResponse wraps successful endpoint payload with status and type metadata.
type StandardDataResponse struct {
	StatusCode int         `json:"statusCode"`
	Data       interface{} `json:"data,omitempty"`
	Type       string      `json:"type"`
}

// InterceptorManager provides enterprise middleware chaining for logging, timeout, and response transformation.
type InterceptorManager struct {
	componentType string
	timeout       time.Duration
	logger        *log.StructuredLogger
}

// NewInterceptorManager constructs an InterceptorManager with default 50s timeout.
func NewInterceptorManager(componentType string, timeout ...time.Duration) *InterceptorManager {
	t := 50 * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		t = timeout[0]
	}
	return &InterceptorManager{
		componentType: componentType,
		timeout:       t,
		logger:        log.CreateLogger("InterceptorManager"),
	}
}

// LoggingMiddleware logs request initiation and completion with correlation ID and duration in ms.
func (m *InterceptorManager) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		corrID := r.Header.Get("x-correlation-id")
		if corrID == "" {
			corrID = r.Header.Get("x-request-id")
		}
		if corrID == "" {
			corrID = uuid.New().String()
		}

		ctx := context.WithValue(r.Context(), "correlation_id", corrID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Correlation-ID", corrID)
		w.Header().Set("X-Request-ID", corrID)

		m.logger.Debug(log.LogArguments{
			Message:     fmt.Sprintf("[%s] Request started: %s %s", corrID, r.Method, r.URL.Path),
			Context:     "HTTP Request",
			ServiceName: m.componentType,
			Metadata: map[string]interface{}{
				"method":        r.Method,
				"url":           r.URL.String(),
				"correlationId": corrID,
			},
		})

		rw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		m.logger.Debug(log.LogArguments{
			Message:     fmt.Sprintf("[%s] Request finished: %s %s in %dms", corrID, r.Method, r.URL.Path, duration.Milliseconds()),
			Context:     "HTTP Request",
			ServiceName: m.componentType,
			Metadata: map[string]interface{}{
				"method":        r.Method,
				"url":           r.URL.String(),
				"correlationId": corrID,
				"durationMs":    duration.Milliseconds(),
				"status":        rw.statusCode,
			},
		})
	})
}

// TimeoutMiddleware bounds execution time with context deadline cancellation.
func (m *InterceptorManager) TimeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), m.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TransformResponse formats and serializes data into the standard response envelope.
func (m *InterceptorManager) TransformResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr && val.IsNil() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	typeName := reflect.TypeOf(data).String()
	typeName = strings.TrimPrefix(typeName, "*")

	resp := StandardDataResponse{
		StatusCode: statusCode,
		Data:       data,
		Type:       typeName,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *statusResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
