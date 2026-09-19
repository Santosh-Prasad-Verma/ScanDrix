package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

type contextKey string

const (
	currentSpanKey        contextKey = "scandrix.current_span"
	currentSpanContextKey contextKey = "scandrix.current_span_context"
	currentBaggageKey     contextKey = "scandrix.current_baggage"
)

const (
	TraceParentHeader = "traceparent"
	TraceStateHeader  = "tracestate"
	BaggageHeader     = "baggage"
)

// GenerateTraceID produces a 32-character hexadecimal OpenTelemetry trace ID (16 bytes).
func GenerateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateSpanID produces a 16-character hexadecimal OpenTelemetry span ID (8 bytes).
func GenerateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ContextWithSpan stores an active Span in the context.
func ContextWithSpan(ctx context.Context, s Span) context.Context {
	if s == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, currentSpanKey, s)
	return context.WithValue(ctx, currentSpanContextKey, s.SpanContext())
}

// SpanFromContext retrieves the active Span from the context.
func SpanFromContext(ctx context.Context) Span {
	if ctx == nil {
		return nil
	}
	if s, ok := ctx.Value(currentSpanKey).(Span); ok {
		return s
	}
	return nil
}

// ContextWithSpanContext stores an explicit SpanContext in the context.
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, currentSpanContextKey, sc)
}

// SpanContextFromContext extracts the SpanContext from context if available.
func SpanContextFromContext(ctx context.Context) (SpanContext, bool) {
	if ctx == nil {
		return SpanContext{}, false
	}
	if sc, ok := ctx.Value(currentSpanContextKey).(SpanContext); ok && sc.IsValid() {
		return sc, true
	}
	if s, ok := ctx.Value(currentSpanKey).(Span); ok && s != nil {
		sc := s.SpanContext()
		if sc.IsValid() {
			return sc, true
		}
	}
	return SpanContext{}, false
}

// Baggage holds distributed contextual key-value pairs.
type Baggage map[string]string

// ContextWithBaggage attaches baggage key-values to the context.
func ContextWithBaggage(ctx context.Context, bag Baggage) context.Context {
	existing := BaggageFromContext(ctx)
	merged := make(Baggage, len(existing)+len(bag))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range bag {
		merged[k] = v
	}
	return context.WithValue(ctx, currentBaggageKey, merged)
}

// BaggageFromContext retrieves baggage map from context.
func BaggageFromContext(ctx context.Context) Baggage {
	if ctx == nil {
		return Baggage{}
	}
	if b, ok := ctx.Value(currentBaggageKey).(Baggage); ok {
		return b
	}
	return Baggage{}
}

// W3CTraceContextPropagator injects and extracts W3C TraceContext headers.
type W3CTraceContextPropagator struct{}

// Inject serializes SpanContext and Baggage into string headers map.
func (p W3CTraceContextPropagator) Inject(ctx context.Context, carrier map[string]string) {
	if carrier == nil {
		return
	}
	sc, ok := SpanContextFromContext(ctx)
	if ok && sc.IsValid() {
		// Format: 00-{trace_id}-{span_id}-{trace_flags}
		flags := fmt.Sprintf("%02x", sc.TraceFlags)
		carrier[TraceParentHeader] = fmt.Sprintf("00-%s-%s-%s", sc.TraceID, sc.SpanID, flags)
		if sc.TraceState != "" {
			carrier[TraceStateHeader] = sc.TraceState
		}
	}

	bag := BaggageFromContext(ctx)
	if len(bag) > 0 {
		var parts []string
		for k, v := range bag {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		carrier[BaggageHeader] = strings.Join(parts, ",")
	}
}

// Extract parses W3C TraceContext and Baggage headers from carrier map into context.
func (p W3CTraceContextPropagator) Extract(ctx context.Context, carrier map[string]string) context.Context {
	if carrier == nil {
		return ctx
	}

	tp := carrier[TraceParentHeader]
	if tp == "" {
		// Case insensitive fallback
		for k, v := range carrier {
			if strings.EqualFold(k, TraceParentHeader) {
				tp = v
				break
			}
		}
	}

	if tp != "" {
		parts := strings.Split(tp, "-")
		if len(parts) >= 4 && parts[0] == "00" && len(parts[1]) == 32 && len(parts[2]) == 16 {
			flags, _ := strconv.ParseUint(parts[3], 16, 8)
			sc := SpanContext{
				TraceID:    parts[1],
				SpanID:     parts[2],
				TraceFlags: byte(flags),
				Remote:     true,
			}
			for k, v := range carrier {
				if strings.EqualFold(k, TraceStateHeader) {
					sc.TraceState = v
					break
				}
			}
			ctx = ContextWithSpanContext(ctx, sc)
		}
	}

	var rawBaggage string
	for k, v := range carrier {
		if strings.EqualFold(k, BaggageHeader) {
			rawBaggage = v
			break
		}
	}

	if rawBaggage != "" {
		bag := make(Baggage)
		for _, pair := range strings.Split(rawBaggage, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				bag[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		if len(bag) > 0 {
			ctx = ContextWithBaggage(ctx, bag)
		}
	}

	return ctx
}
