package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTracerAndSpanLifecycle(t *testing.T) {
	memExp := NewMemoryExporter()
	batchProc := NewBatchSpanProcessor(memExp, func(c *BatchProcessorConfig) {
		c.FlushInterval = 50 * time.Millisecond
		c.MaxBatchSize = 10
	})

	sanitizer := NewSanitizationProcessor()
	sanitizer.SetNext(batchProc)

	tracer := NewTracer("test-tracer")
	tracer.AddProcessor(sanitizer)

	ctx := context.Background()
	ctx, root := tracer.Start(ctx, "root-operation", WithSpanKind(SpanKindServer))
	root.SetAttribute("env", "staging")
	root.SetAttribute("api_key", "sk-live-secret-super-long-token-9988776655")

	if !root.IsRecording() {
		t.Fatalf("expected root span to be recording")
	}

	sc := root.SpanContext()
	if !sc.IsValid() {
		t.Fatalf("expected valid span context, got traceID=%s, spanID=%s", sc.TraceID, sc.SpanID)
	}

	// Create child span
	_, child := tracer.Start(ctx, "child-operation", WithSpanKind(SpanKindInternal))
	child.SetAttribute("component", "parser")
	child.RecordError(errors.New("syntax error at line 42"))
	child.End()

	root.End()

	// Wait for batch flush
	time.Sleep(120 * time.Millisecond)

	spans := memExp.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 exported spans, got %d", len(spans))
	}

	// Find root span and check sanitization
	var foundRoot *SpanData
	for _, s := range spans {
		if s.Name == "root-operation" {
			foundRoot = s
			break
		}
	}
	if foundRoot == nil {
		t.Fatalf("root span not found in exported spans")
	}

	redactedKey, ok := foundRoot.Attributes["api_key"].(string)
	if !ok || redactedKey != "[REDACTED]" {
		t.Fatalf("expected api_key to be [REDACTED], got %v", foundRoot.Attributes["api_key"])
	}

	// Verify child span parent linkage
	var foundChild *SpanData
	for _, s := range spans {
		if s.Name == "child-operation" {
			foundChild = s
			break
		}
	}
	if foundChild == nil {
		t.Fatalf("child span not found in exported spans")
	}
	if foundChild.ParentSpanID != sc.SpanID {
		t.Fatalf("expected child.ParentSpanID=%s, got %s", sc.SpanID, foundChild.ParentSpanID)
	}
	if foundChild.Status.Code != StatusError {
		t.Fatalf("expected child status to be error, got %s", foundChild.Status.Code)
	}
}

func TestW3CContextPropagation(t *testing.T) {
	propagator := W3CTraceContextPropagator{}
	carrier := make(map[string]string)

	traceID := GenerateTraceID()
	spanID := GenerateSpanID()
	sc := SpanContext{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: 0x01,
	}

	ctx := ContextWithSpanContext(context.Background(), sc)
	ctx = ContextWithBaggage(ctx, Baggage{
		"tenant_id":    "org_123",
		"workspace_id": "ws_456",
	})

	propagator.Inject(ctx, carrier)

	if carrier[TraceParentHeader] != "00-"+traceID+"-"+spanID+"-01" {
		t.Fatalf("unexpected traceparent: %s", carrier[TraceParentHeader])
	}
	if carrier[BaggageHeader] == "" {
		t.Fatalf("expected non-empty baggage header")
	}

	// Extract back into new context
	extractedCtx := propagator.Extract(context.Background(), carrier)
	extractedSC, ok := SpanContextFromContext(extractedCtx)
	if !ok || !extractedSC.IsValid() {
		t.Fatalf("failed to extract span context from carrier")
	}
	if extractedSC.TraceID != traceID || extractedSC.SpanID != spanID {
		t.Fatalf("extracted sc mismatch: traceID=%s spanID=%s", extractedSC.TraceID, extractedSC.SpanID)
	}

	extractedBaggage := BaggageFromContext(extractedCtx)
	if extractedBaggage["tenant_id"] != "org_123" || extractedBaggage["workspace_id"] != "ws_456" {
		t.Fatalf("extracted baggage mismatch: %v", extractedBaggage)
	}
}

func TestSanitizationDeepRecursiveAndJSON(t *testing.T) {
	sanitizer := NewSanitizationProcessor()

	// Nested JSON in string
	jsonString := `{"user":{"token":"ghp_123456789012345678901234567890123456","name":"alice"}}`
	sanitizedString := sanitizer.SanitizeString(jsonString)

	if sanitizedString == jsonString {
		t.Fatalf("expected JSON string to be sanitized, got original: %s", sanitizedString)
	}

	// Map sanitization
	data := map[string]any{
		"normal_field": "hello world",
		"Password":     "supersecret123",
		"nested": map[string]any{
			"authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			"webhook_secret": "whsec_abcdef123456",
		},
	}

	sanitizedMap := sanitizer.SanitizeMap(data)
	if sanitizedMap["Password"] != "[REDACTED]" {
		t.Fatalf("expected Password to be redacted, got %v", sanitizedMap["Password"])
	}

	nested := sanitizedMap["nested"].(map[string]any)
	if nested["authorization"] != "[REDACTED]" {
		t.Fatalf("expected authorization to be redacted, got %v", nested["authorization"])
	}
	if nested["webhook_secret"] != "[REDACTED]" {
		t.Fatalf("expected webhook_secret to be redacted, got %v", nested["webhook_secret"])
	}
}

func TestReviewFlowTrackerDAG(t *testing.T) {
	tracker := NewReviewFlowTracker()
	flow := tracker.StartFlow("rev_001", "tenant_a", "ws_a", "repo_a", 101, "corr_999")

	flow.RecordStage(StageValidatePayload, time.Now().Add(-50*time.Millisecond), StatusOK, 100, 20, nil, nil)
	flow.RecordStage(StageFetchGitDiff, time.Now().Add(-30*time.Millisecond), StatusOK, 50, 10, nil, nil)
	flow.RecordStage(StageSynthesizeReview, time.Now().Add(-10*time.Millisecond), StatusOK, 500, 200, nil, nil)
	flow.Complete()

	metrics := tracker.GetAggregateMetrics()
	if metrics.TotalReviews != 1 {
		t.Fatalf("expected 1 total review, got %d", metrics.TotalReviews)
	}
	if metrics.SuccessfulReviews != 1 {
		t.Fatalf("expected 1 successful review, got %d", metrics.SuccessfulReviews)
	}
	if metrics.TotalTokensSpent != (120 + 60 + 700) {
		t.Fatalf("expected 880 tokens spent, got %d", metrics.TotalTokensSpent)
	}
}

func TestConcurrentTracingRace(t *testing.T) {
	memExp := NewMemoryExporter()
	batchProc := NewBatchSpanProcessor(memExp, func(c *BatchProcessorConfig) {
		c.FlushInterval = 20 * time.Millisecond
		c.MaxBatchSize = 50
	})
	tracer := NewTracer("concurrent-tracer")
	tracer.AddProcessor(batchProc)

	var wg sync.WaitGroup
	workers := 20
	spansPerWorker := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < spansPerWorker; j++ {
				_, span := tracer.Start(context.Background(), "concurrent-op")
				span.SetAttribute("worker", workerID)
				span.SetAttribute("iter", j)
				span.AddEvent("tick", map[string]any{"timestamp": time.Now().UnixNano()})
				span.End()
			}
		}(i)
	}

	wg.Wait()
	_ = batchProc.Shutdown(context.Background())

	spans := memExp.GetSpans()
	if len(spans) != workers*spansPerWorker {
		t.Fatalf("expected %d spans, got %d", workers*spansPerWorker, len(spans))
	}
}
