package observability

import (
	"context"
	"time"
)

// SpanKind defines the relationship between the span and its initiating context.
type SpanKind string

const (
	SpanKindInternal SpanKind = "internal"
	SpanKindServer   SpanKind = "server"
	SpanKindClient   SpanKind = "client"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
)

// StatusCode indicates the operational outcome of a span.
type StatusCode string

const (
	StatusUnset StatusCode = "unset"
	StatusOK    StatusCode = "ok"
	StatusError StatusCode = "error"
)

// SpanStatus encapsulates execution status and optional descriptive messages.
type SpanStatus struct {
	Code        StatusCode `json:"code"`
	Description string     `json:"description,omitempty"`
}

// SpanContext carries immutable identifiers across process boundaries.
type SpanContext struct {
	TraceID    string `json:"traceId"`
	SpanID     string `json:"spanId"`
	TraceFlags byte   `json:"traceFlags"`
	TraceState string `json:"traceState,omitempty"`
	Remote     bool   `json:"remote"`
}

// IsValid checks if the SpanContext has valid 32-hex TraceID and 16-hex SpanID.
func (sc SpanContext) IsValid() bool {
	return len(sc.TraceID) == 32 && len(sc.SpanID) == 16
}

// SpanEvent represents a timestamped milestone during span execution.
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanLink represents a causal link to another span.
type SpanLink struct {
	Context    SpanContext    `json:"context"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// SpanOptions configures span creation.
type SpanOptions struct {
	Kind       SpanKind
	StartTime  time.Time
	Attributes map[string]any
	Links      []SpanLink
	Parent     *SpanContext
}

// SpanData is an immutable snapshot of a completed span ready for export.
type SpanData struct {
	Name         string            `json:"name"`
	Context      SpanContext       `json:"context"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Kind         SpanKind          `json:"kind"`
	StartTime    time.Time         `json:"startTime"`
	EndTime      time.Time         `json:"endTime"`
	DurationMs   int64             `json:"durationMs"`
	Attributes   map[string]any    `json:"attributes"`
	Events       []SpanEvent       `json:"events"`
	Links        []SpanLink        `json:"links"`
	Status       SpanStatus        `json:"status"`
	Resource     map[string]string `json:"resource"`
}

// Span is the active tracing interface.
type Span interface {
	SpanContext() SpanContext
	IsRecording() bool
	SetAttribute(key string, value any) Span
	SetAttributes(attributes map[string]any) Span
	SetStatus(code StatusCode, description string) Span
	RecordError(err error) Span
	AddEvent(name string, attributes map[string]any) Span
	End()
	Snapshot() *SpanData
}

// SpanProcessor processes spans during their lifecycle.
type SpanProcessor interface {
	OnStart(parent context.Context, span Span)
	OnEnd(span *SpanData)
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// SpanExporter exports completed spans to external telemetry backends.
type SpanExporter interface {
	Export(ctx context.Context, spans []*SpanData) error
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

// Tracer defines the tracer interface for creating spans.
type Tracer interface {
	Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)
}

// SpanOption is a functional option for configuring a span.
type SpanOption func(*SpanOptions)

// WithSpanKind sets the span kind.
func WithSpanKind(kind SpanKind) SpanOption {
	return func(o *SpanOptions) {
		o.Kind = kind
	}
}

// WithStartTime sets the explicit span start time.
func WithStartTime(t time.Time) SpanOption {
	return func(o *SpanOptions) {
		o.StartTime = t
	}
}

// WithAttributes adds initial attributes to the span.
func WithAttributes(attrs map[string]any) SpanOption {
	return func(o *SpanOptions) {
		if o.Attributes == nil {
			o.Attributes = make(map[string]any, len(attrs))
		}
		for k, v := range attrs {
			o.Attributes[k] = v
		}
	}
}

// WithParent sets the explicit parent span context.
func WithParent(parent SpanContext) SpanOption {
	return func(o *SpanOptions) {
		o.Parent = &parent
	}
}
