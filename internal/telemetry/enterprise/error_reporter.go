package enterprise

import (
	"context"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	awsKeyRegex    = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	bearerRegex    = regexp.MustCompile(`(?i)Bearer\s+[a-zA-Z0-9_\-\.]{16,}`)
	secretParamRegex = regexp.MustCompile(`(?i)(password|secret|token|api_key)=([^\s&]+)`)
)

// ErrorReporter captures, sanitises, and routes exceptions to Sentry or internal audit stores.
type ErrorReporter struct {
	mu      sync.RWMutex
	reports []ErrorReport
}

// NewErrorReporter initializes the error reporter.
func NewErrorReporter() *ErrorReporter {
	return &ErrorReporter{
		reports: make([]ErrorReport, 0),
	}
}

// CaptureError records an error event, stripping any accidentally included secrets.
func (r *ErrorReporter) CaptureError(ctx context.Context, err error, extra map[string]any) *ErrorReport {
	if err == nil {
		return nil
	}

	sanitizedMessage := ScrubSecrets(err.Error())

	// Capture stack trace
	rawStack := string(debug.Stack())
	sanitizedStack := ScrubSecrets(rawStack)
	stackLines := strings.Split(sanitizedStack, "\n")

	cleanExtra := make(map[string]any)
	for k, v := range extra {
		if strVal, ok := v.(string); ok {
			cleanExtra[k] = ScrubSecrets(strVal)
		} else {
			cleanExtra[k] = v
		}
	}

	report := &ErrorReport{
		EventID:    uuid.New(),
		Timestamp:  time.Now().UTC(),
		Level:      "error",
		Message:    sanitizedMessage,
		StackTrace: stackLines,
		Tags: map[string]string{
			"runtime": "go",
		},
		Extra: cleanExtra,
	}

	r.mu.Lock()
	r.reports = append(r.reports, *report)
	r.mu.Unlock()

	return report
}

// ScrubSecrets ensures API keys, tokens, and credentials are redacted (Master Rule 1.7).
func ScrubSecrets(input string) string {
	// 1. Scrub AWS keys
	res := awsKeyRegex.ReplaceAllString(input, "[REDACTED_AWS_KEY]")

	// 2. Scrub Bearer tokens
	res = bearerRegex.ReplaceAllString(res, "Bearer [REDACTED_TOKEN]")

	// 3. Scrub URI / query secrets
	res = secretParamRegex.ReplaceAllString(res, "$1=[REDACTED_VALUE]")

	return res
}

// GetReports retrieves recorded error reports for validation or dispatch.
func (r *ErrorReporter) GetReports() []ErrorReport {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]ErrorReport, len(r.reports))
	copy(res, r.reports)
	return res
}
