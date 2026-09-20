package log

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	sensitiveKeyRegex = regexp.MustCompile(`(?i)(password|secret|token|api_?key|auth|bearer|private_?key|credit_?card|cvv)`)
	jwtRegex          = regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`)
)

// ObservabilityService provides thread-safe context resolution, correlation ID management,
// and deep sanitization of all log payloads to prevent secret leakage.
type ObservabilityService struct {
	mu           sync.RWMutex
	contextStore map[string]string
	logger       *StructuredLogger
}

// NewObservabilityService constructs a new ObservabilityService.
func NewObservabilityService() *ObservabilityService {
	return &ObservabilityService{
		contextStore: make(map[string]string),
		logger:       CreateLogger("ObservabilityService"),
	}
}

// SetCorrelationID associates a correlation ID with a specific request or worker key.
func (s *ObservabilityService) SetCorrelationID(key, correlationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.contextStore[key] = correlationID
}

// GetCorrelationID retrieves the correlation ID for a key.
func (s *ObservabilityService) GetCorrelationID(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.contextStore[key]
}

// ClearContext removes an entry from the context store.
func (s *ObservabilityService) ClearContext(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.contextStore, key)
}

// SanitizeMetadata recursively scrubs sensitive tokens, passwords, and private keys from maps.
func (s *ObservabilityService) SanitizeMetadata(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	clean := make(map[string]interface{}, len(data))
	for k, v := range data {
		if sensitiveKeyRegex.MatchString(k) {
			clean[k] = "[REDACTED]"
			continue
		}

		switch val := v.(type) {
		case string:
			clean[k] = s.SanitizeString(val)
		case map[string]interface{}:
			clean[k] = s.SanitizeMetadata(val)
		case []interface{}:
			cleanSlice := make([]interface{}, len(val))
			for i, elem := range val {
				if elemMap, ok := elem.(map[string]interface{}); ok {
					cleanSlice[i] = s.SanitizeMetadata(elemMap)
				} else if str, ok := elem.(string); ok {
					cleanSlice[i] = s.SanitizeString(str)
				} else {
					cleanSlice[i] = elem
				}
			}
			clean[k] = cleanSlice
		default:
			clean[k] = v
		}
	}
	return clean
}

// SanitizeString redacts JWT tokens and sensitive substrings.
func (s *ObservabilityService) SanitizeString(val string) string {
	if jwtRegex.MatchString(val) {
		return jwtRegex.ReplaceAllString(val, "[REDACTED_JWT]")
	}
	if strings.HasPrefix(strings.ToLower(val), "bearer ") {
		return "Bearer [REDACTED]"
	}
	return val
}

// StartTimingProbe returns a function that calculates elapsed duration and logs it.
func (s *ObservabilityService) StartTimingProbe(opName string) func() time.Duration {
	start := time.Now()
	return func() time.Duration {
		elapsed := time.Since(start)
		s.logger.Debug(LogArguments{
			Message: opName + " completed",
			Context: "TimingProbe",
			Metadata: map[string]interface{}{
				"operation": opName,
				"durationMs": elapsed.Milliseconds(),
			},
		})
		return elapsed
	}
}

// ContextWithCorrelation returns a new context injected with the given correlation ID.
func (s *ObservabilityService) ContextWithCorrelation(parent context.Context, correlationID string) context.Context {
	return context.WithValue(parent, "correlation_id", correlationID)
}
