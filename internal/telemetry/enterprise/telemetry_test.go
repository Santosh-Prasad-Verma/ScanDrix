package enterprise_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/telemetry/enterprise"
)

func TestEnterpriseTelemetrySuite(t *testing.T) {
	ctx := context.Background()

	// 1. Test W3C Trace Context Propagation
	tracer := enterprise.NewTracer()

	// Root Span
	rootCtx, rootSpan := tracer.StartSpan(ctx, "review.orchestration", map[string]any{
		"workspace": "ws-123",
	})
	if len(rootSpan.TraceID) != 32 || len(rootSpan.ID) != 16 || rootSpan.ParentSpanID != "" {
		t.Fatalf("unexpected root span: %+v", rootSpan)
	}

	// Child Span
	_, childSpan := tracer.StartSpan(rootCtx, "ast.parser", nil)
	if childSpan.TraceID != rootSpan.TraceID || childSpan.ParentSpanID != rootSpan.ID {
		t.Fatalf("child span must inherit trace_id and point to parent span_id: %+v", childSpan)
	}

	tracer.EndSpan(childSpan, "OK", "")
	tracer.EndSpan(rootSpan, "OK", "")

	recorded := tracer.GetRecordedSpans()
	if len(recorded) != 2 {
		t.Fatalf("expected 2 recorded spans, got %d", len(recorded))
	}

	// 2. Test W3C Header Parsing
	rawHeader := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tc, err := enterprise.ExtractW3C(rawHeader)
	if err != nil || tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || tc.SpanID != "00f067aa0ba902b7" {
		t.Fatalf("w3c extract failed: %+v, err: %v", tc, err)
	}
	formatted := enterprise.FormatW3C(tc)
	if formatted != rawHeader {
		t.Fatalf("expected %s, got %s", rawHeader, formatted)
	}

	// 3. Test Continuous Profiler Snapshot
	profiler := enterprise.NewProfiler(enterprise.ProfilerConfig{
		ServiceName: "scandrix-worker",
		Environment: "production",
	})
	stats, pprofBytes, err := profiler.CaptureSnapshot()
	if err != nil || len(pprofBytes) == 0 {
		t.Fatalf("capture snapshot failed: err=%v, bytes=%d", err, len(pprofBytes))
	}
	if stats["service"] != "scandrix-worker" || stats["goroutines"].(int) <= 0 {
		t.Fatalf("unexpected profiler stats: %+v", stats)
	}

	// 4. Test Heartbeat Ping to Mock Server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	heartbeat := enterprise.NewHeartbeatRunner(enterprise.HeartbeatConfig{
		URL:     mockServer.URL,
		Timeout: 2 * time.Second,
	})

	if err := heartbeat.Ping(ctx); err != nil {
		t.Fatalf("heartbeat ping failed: %v", err)
	}
	successes, failures, lastPing := heartbeat.Stats()
	if successes != 1 || failures != 0 || lastPing == nil {
		t.Fatalf("unexpected heartbeat stats: success=%d, fail=%d", successes, failures)
	}

	// 5. Test Error Reporter Secret Scrubbing (Master Rule 1.7)
	reporter := enterprise.NewErrorReporter()
	leakedErr := errors.New("upstream failed for key AKIAIOSFODNN7EXAMPLE and Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz token")
	report := reporter.CaptureError(ctx, leakedErr, map[string]any{
		"db_url": "postgres://user:password=supersecret@localhost:5432/db",
	})

	if strings.Contains(report.Message, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("CRITICAL SECURITY VIOLATION: AWS key not scrubbed: %s", report.Message)
	}
	if strings.Contains(report.Message, "eyJhbGciOiJIUzI1Ni") {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Bearer token not scrubbed: %s", report.Message)
	}
	if !strings.Contains(report.Message, "[REDACTED_AWS_KEY]") || !strings.Contains(report.Message, "[REDACTED_TOKEN]") {
		t.Fatalf("expected redactions in message: %s", report.Message)
	}
	if strings.Contains(report.Extra["db_url"].(string), "supersecret") {
		t.Fatalf("CRITICAL SECURITY VIOLATION: password not scrubbed from extra attributes")
	}
}
