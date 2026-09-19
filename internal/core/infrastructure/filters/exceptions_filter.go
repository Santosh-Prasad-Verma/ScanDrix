package filters

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/core/infrastructure/config"
	"github.com/scandrix/backend/internal/core/log"
)

// StandardErrorResponse defines the unified API error envelope returned by all HTTP endpoints.
type StandardErrorResponse struct {
	StatusCode int         `json:"statusCode"`
	Timestamp  string      `json:"timestamp"`
	Path       string      `json:"path"`
	Error      string      `json:"error"`
	Message    string      `json:"message"`
	ErrorKey   string      `json:"error_key,omitempty"`
	Code       string      `json:"code,omitempty"`
	Details    interface{} `json:"details,omitempty"`
}

// ExceptionsFilter inspects, classifies, and formats errors across HTTP and RPC request boundaries.
type ExceptionsFilter struct {
	componentType string
	logger        *log.StructuredLogger
}

// NewExceptionsFilter constructs a new ExceptionsFilter.
func NewExceptionsFilter(componentType string) *ExceptionsFilter {
	return &ExceptionsFilter{
		componentType: componentType,
		logger:        log.CreateLogger("ExceptionsFilter"),
	}
}

// HandleHTTP catches an error, calculates the appropriate HTTP status code,
// reports 5xx server faults to Sentry, and writes the standardized JSON response.
func (f *ExceptionsFilter) HandleHTTP(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	statusCode := http.StatusInternalServerError
	var apiErr APIError
	var customErr CustomDomainException

	isPgInvalidInput := strings.Contains(err.Error(), "22P02") || strings.Contains(err.Error(), "invalid input syntax")

	if isPgInvalidInput {
		statusCode = http.StatusBadRequest
	} else if errors.As(err, &customErr) {
		statusCode = customErr.StatusCode()
	} else if errors.As(err, &apiErr) {
		statusCode = apiErr.StatusCode()
	}

	path := r.URL.Path
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = "unknown-req-id"
	}

	// Sentry reporting for 5xx errors
	if statusCode >= 500 && !isPgInvalidInput {
		config.ReportExceptionToSentry(err, "ExceptionsFilter", map[string]string{
			"requestId":  reqID,
			"statusCode": http.StatusText(statusCode),
		}, map[string]interface{}{
			"path":   path,
			"method": r.Method,
		})
	}

	// Logging: 4xx as Warn, 5xx as Error
	msg := err.Error()
	if isPgInvalidInput {
		msg = "Invalid parameter format"
	}

	logArgs := log.LogArguments{
		Message:     "[" + http.StatusText(statusCode) + "] " + msg,
		Context:     "ExceptionsFilter",
		ServiceName: f.componentType,
		Metadata: map[string]interface{}{
			"path":      path,
			"method":    r.Method,
			"status":    statusCode,
			"requestId": reqID,
		},
	}

	if statusCode >= 500 {
		logArgs.Error = err
		f.logger.Error(logArgs)
	} else {
		f.logger.Warn(logArgs)
	}

	resp := StandardErrorResponse{
		StatusCode: statusCode,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Path:       path,
		Error:      http.StatusText(statusCode),
		Message:    msg,
	}

	if customErr != nil {
		resp.ErrorKey = customErr.ErrorKey()
		resp.Code = customErr.Code()
		resp.Details = customErr.Details()
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}
