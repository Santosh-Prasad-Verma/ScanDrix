package enterprise

import (
	"time"

	"github.com/google/uuid"
)

// TraceContext encapsulates W3C Trace Context specification attributes.
type TraceContext struct {
	TraceID    string `json:"trace_id"`    // 16-byte hex (32 chars)
	SpanID     string `json:"span_id"`     // 8-byte hex (16 chars)
	TraceFlags string `json:"trace_flags"` // 1-byte hex (2 chars, e.g. "01" sampled)
	TraceState string `json:"trace_state"` // vendor-specific key-value pairs
}

// Span records execution duration and contextual metadata for a unit of work.
type Span struct {
	ID           string         `json:"id"`
	TraceID      string         `json:"trace_id"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Name         string         `json:"name"`
	StartTime    time.Time      `json:"start_time"`
	EndTime      time.Time      `json:"end_time"`
	Attributes   map[string]any `json:"attributes"`
	Status       string         `json:"status"` // "OK", "ERROR"
	ErrorMessage string         `json:"error_message,omitempty"`
}

// ProfileType distinguishes profiling dimensions.
type ProfileType string

const (
	ProfileTypeCPU       ProfileType = "cpu"
	ProfileTypeHeap      ProfileType = "heap"
	ProfileTypeGoroutine ProfileType = "goroutine"
	ProfileTypeBlock     ProfileType = "block"
)

// ProfilerConfig specifies Pyroscope / continuous profiling parameters.
type ProfilerConfig struct {
	ServerAddress string        `json:"server_address"`
	ServiceName   string        `json:"service_name"`
	Environment   string        `json:"environment"`
	UploadPeriod  time.Duration `json:"upload_period"`
	AuthToken     string        `json:"auth_token,omitempty"`
}

// HeartbeatConfig controls BetterStack heartbeat pinging.
type HeartbeatConfig struct {
	URL      string        `json:"url"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
}

// ErrorReport captures sanitised exception telemetry.
type ErrorReport struct {
	EventID    uuid.UUID         `json:"event_id"`
	Timestamp  time.Time         `json:"timestamp"`
	Level      string            `json:"level"` // "error", "fatal", "warning"
	Message    string            `json:"message"`
	StackTrace []string          `json:"stack_trace"`
	Tags       map[string]string `json:"tags"`
	Extra      map[string]any    `json:"extra"`
}
