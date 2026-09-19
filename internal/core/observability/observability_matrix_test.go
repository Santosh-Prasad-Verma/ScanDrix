package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Sanitizer Matrix Tests
// ============================================================================

func TestSanitizerMatrix_ComprehensiveScrubbingPatterns(t *testing.T) {
	processor := NewSanitizationProcessor()

	testCases := []struct {
		name          string
		rawInput      string
		expectedMatch bool
		subMatch      string
	}{
		{
			name:          "OpenAI Legacy Key",
			rawInput:      "Bearer sk-1234567890abcdef1234567890abcdef",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "OpenAI Modern Key",
			rawInput:      "Key: sk-proj-1234567890abcdef1234567890abcdef",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "Anthropic Key",
			rawInput:      "anthropic-key-1234567890abcdef1234567890abcdef",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "GitHub Personal Access Token",
			rawInput:      "ghp_123456789012345678901234567890123456",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "GitLab Personal Access Token",
			rawInput:      "glpat-1234567890abcdef1234567890abcdef",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "ScanDrix Team Live API Key",
			rawInput:      "scandrix_live_abcdef0123456789abcdef0123456789",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "JWT Auth Token In Value",
			rawInput:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnopqrstuvwxyz1234567890",
			expectedMatch: true,
			subMatch:      "[REDACTED]",
		},
		{
			name:          "Standard Safe String Without Secrets",
			rawInput:      "package main; import fmt; func main() { fmt.Println(42) }",
			expectedMatch: false,
			subMatch:      "package main",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sanitized := processor.SanitizeString(tc.rawInput)
			if tc.expectedMatch {
				assert.Contains(t, sanitized, tc.subMatch)
				assert.NotContains(t, sanitized, "1234567890abcdef")
			} else {
				assert.Equal(t, tc.rawInput, sanitized)
			}
		})
	}
}

func TestSanitizerMatrix_KeyBasedScrubbingAndStyles(t *testing.T) {
	t.Run("Default Sensitive Key Name Matches", func(t *testing.T) {
		proc := NewSanitizationProcessor()

		payload := map[string]any{
			"username":      "alice",
			"password":      "SuperSecretP@ssword123",
			"api_key":       "my-custom-unformatted-key",
			"client_secret": "sensitive-oauth-secret",
			"refresh_token": "rt_987654321",
			"nested": map[string]any{
				"private_key": "MIIEvgIBADANBgkqhkiG9w0BAQEFAASC...",
				"safe_field":  "public-metadata-flag",
			},
		}

		sanitized := proc.SanitizeMap(payload)
		assert.Equal(t, "alice", sanitized["username"])
		assert.Equal(t, "[REDACTED]", sanitized["password"])
		assert.Equal(t, "[REDACTED]", sanitized["api_key"])
		assert.Equal(t, "[REDACTED]", sanitized["client_secret"])
		assert.Equal(t, "[REDACTED]", sanitized["refresh_token"])

		nested, ok := sanitized["nested"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "[REDACTED]", nested["private_key"])
		assert.Equal(t, "public-metadata-flag", nested["safe_field"])
	})

	t.Run("Partial Redaction Style Preserves Prefix/Suffix", func(t *testing.T) {
		proc := NewSanitizationProcessor(func(cfg *SanitizationConfig) {
			cfg.RedactionStyle = RedactionStylePartial
			cfg.RedactionToken = "..."
		})

		payload := map[string]any{
			"api_key": "sk-1234567890abcdef1234567890abcdef",
		}
		sanitized := proc.SanitizeMap(payload)
		apiKeyVal, ok := sanitized["api_key"].(string)
		require.True(t, ok)
		assert.Contains(t, apiKeyVal, "sk-")
		assert.Contains(t, apiKeyVal, "...")
		assert.NotEqual(t, "sk-1234567890abcdef1234567890abcdef", apiKeyVal)
	})

	t.Run("Custom Sensitive Key Extension", func(t *testing.T) {
		proc := NewSanitizationProcessor(func(cfg *SanitizationConfig) {
			cfg.SensitiveKeys = append(cfg.SensitiveKeys, "webhook_signature", "stripe_customer_token")
		})

		payload := map[string]any{
			"webhook_signature":     "sha256=abcdef0123456789",
			"stripe_customer_token": "cus_123456789",
			"normal_field":          "visible",
		}

		sanitized := proc.SanitizeMap(payload)
		assert.Equal(t, "[REDACTED]", sanitized["webhook_signature"])
		assert.Equal(t, "[REDACTED]", sanitized["stripe_customer_token"])
		assert.Equal(t, "visible", sanitized["normal_field"])
	})
}

func TestSanitizerMatrix_DeepRecursionAndJSONSafety(t *testing.T) {
	proc := NewSanitizationProcessor(func(cfg *SanitizationConfig) {
		cfg.MaxDepth = 8
	})

	t.Run("Deeply Nested Map Respects MaxDepth", func(t *testing.T) {
		root := make(map[string]any)
		curr := root
		for i := 0; i < 20; i++ {
			next := make(map[string]any)
			curr[fmt.Sprintf("level_%d", i)] = next
			curr = next
		}
		curr["deep_key"] = "deep_value"

		// Should not stack overflow or crash
		sanitized := proc.SanitizeMap(root)
		require.NotNil(t, sanitized)
	})

	t.Run("Valid JSON String with Mixed Secrets", func(t *testing.T) {
		rawJSON := `{
			"service": "ScanDrix Review Engine",
			"auth": {
				"apiKey": "sk-1234567890abcdef1234567890abcdef",
				"password": "production_password_1"
			},
			"metrics": [
				{"rule": "security/sql_injection", "score": 9.8}
			]
		}`

		scrubbedJSON := proc.SanitizeString(rawJSON)
		assert.NotContains(t, scrubbedJSON, "production_password_1")
		assert.NotContains(t, scrubbedJSON, "sk-1234567890abcdef")
		assert.Contains(t, scrubbedJSON, "ScanDrix Review Engine")
		assert.Contains(t, scrubbedJSON, "security/sql_injection")

		// Verify valid JSON output
		var parsed map[string]any
		err := json.Unmarshal([]byte(scrubbedJSON), &parsed)
		require.NoError(t, err)
	})

	t.Run("Invalid JSON String Falls Back to Regex Scrub", func(t *testing.T) {
		invalidJSON := `not valid json at all, but contains sk-1234567890abcdef1234567890abcdef here!`
		scrubbed := proc.SanitizeString(invalidJSON)
		assert.Contains(t, scrubbed, "[REDACTED]")
		assert.NotContains(t, scrubbed, "sk-1234567890abcdef")
	})
}

// ============================================================================
// W3C Context Propagation Matrix Tests
// ============================================================================

func TestSpanMatrix_W3CContextSerializationAndParsing(t *testing.T) {
	propagator := W3CTraceContextPropagator{}

	t.Run("Valid W3C TraceContext Round-Trip", func(t *testing.T) {
		traceID := GenerateTraceID()
		spanID := GenerateSpanID()
		sc := SpanContext{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: 0x01,
		}

		ctx := ContextWithSpanContext(context.Background(), sc)
		ctx = ContextWithBaggage(ctx, Baggage{
			"tenant_id":    "tenant-matrix-101",
			"workspace_id": "ws-matrix-202",
		})

		carrier := make(map[string]string)
		propagator.Inject(ctx, carrier)

		expectedTraceParent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
		assert.Equal(t, expectedTraceParent, carrier[TraceParentHeader])
		assert.Contains(t, carrier[BaggageHeader], "tenant_id=tenant-matrix-101")

		// Extract into new context
		extractedCtx := propagator.Extract(context.Background(), carrier)
		extractedSC, ok := SpanContextFromContext(extractedCtx)
		require.True(t, ok)
		assert.True(t, extractedSC.IsValid())
		assert.Equal(t, traceID, extractedSC.TraceID)
		assert.Equal(t, spanID, extractedSC.SpanID)
		assert.Equal(t, byte(0x01), extractedSC.TraceFlags)

		extractedBaggage := BaggageFromContext(extractedCtx)
		assert.Equal(t, "tenant-matrix-101", extractedBaggage["tenant_id"])
		assert.Equal(t, "ws-matrix-202", extractedBaggage["workspace_id"])
	})

	t.Run("Adversarial Malformed Traceparents in Carrier", func(t *testing.T) {
		malformedHeaders := []string{
			"",
			"invalid-header-string",
			"00-short-id-01",
			"00-4bf92f3577b34da6a3ce929d0e0e4736-tooshort-01",
			"01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		}

		for _, h := range malformedHeaders {
			carrier := map[string]string{TraceParentHeader: h}
			extractedCtx := propagator.Extract(context.Background(), carrier)
			extractedSC, ok := SpanContextFromContext(extractedCtx)
			assert.False(t, ok && extractedSC.IsValid(), "Expected invalid context for header: %s", h)
		}
	})
}

// ============================================================================
// Tracer, Span & Hierarchy Matrix Tests
// ============================================================================

func TestSpanMatrix_HierarchicalSpanLifecycleAndAttributes(t *testing.T) {
	memExp := NewMemoryExporter()
	batchProc := NewBatchSpanProcessor(memExp, func(c *BatchProcessorConfig) {
		c.FlushInterval = 20 * time.Millisecond
		c.MaxBatchSize = 10
	})

	tracer := NewTracer("scandrix.core.test")
	tracer.AddProcessor(batchProc)

	ctx := context.Background()

	// 1. Root span
	ctxRoot, rootSpan := tracer.Start(ctx, "ReviewPRWorkflow", WithSpanKind(SpanKindServer))
	rootSpan.SetAttribute(TenantID, "tenant-matrix-101")
	rootSpan.SetAttribute(RepositoryID, "repo-scan-001")
	rootSpan.SetAttribute(PullRequestNumber, 42)
	rootSpan.AddEvent("pr.diff.downloaded", map[string]any{"files_changed": 15})

	// 2. Child span 1: AST Analysis
	_, childSpan1 := tracer.Start(ctxRoot, "ASTAnalysisStage", WithSpanKind(SpanKindInternal))
	childSpan1.SetAttribute("analysis.language", "go")
	childSpan1.SetAttribute("analysis.nodes_parsed", 8500)
	childSpan1.SetStatus(StatusOK, "AST successfully constructed")
	childSpan1.End()

	// 3. Child span 2: Drixy Agent Deliberation
	_, childSpan2 := tracer.Start(ctxRoot, "DrixyDeliberationStage", WithSpanKind(SpanKindClient))
	childSpan2.SetAttribute(GenAISystem, SystemAnthropic)
	childSpan2.SetAttribute(GenAIRequestModel, "claude-3-7-sonnet")
	childSpan2.SetAttribute(GenAIUsageInputTokens, 2400)
	childSpan2.SetAttribute(GenAIUsageOutputTokens, 850)
	childSpan2.RecordError(errors.New("rate limit backoff triggered"))
	childSpan2.SetStatus(StatusError, "transient error handled")
	childSpan2.End()

	// End root span
	rootSpan.SetStatus(StatusOK, "Review completed")
	rootSpan.End()

	// Wait for batch flush
	time.Sleep(100 * time.Millisecond)

	spans := memExp.GetSpans()
	require.Len(t, spans, 3)

	var foundRoot, foundAST, foundDrixy *SpanData
	for _, s := range spans {
		switch s.Name {
		case "ReviewPRWorkflow":
			foundRoot = s
		case "ASTAnalysisStage":
			foundAST = s
		case "DrixyDeliberationStage":
			foundDrixy = s
		}
	}

	require.NotNil(t, foundRoot)
	require.NotNil(t, foundAST)
	require.NotNil(t, foundDrixy)

	// Verify Parent-Child Hierarchy Links
	assert.Empty(t, foundRoot.ParentSpanID)
	assert.Equal(t, foundRoot.Context.SpanID, foundAST.ParentSpanID)
	assert.Equal(t, foundRoot.Context.SpanID, foundDrixy.ParentSpanID)
	assert.Equal(t, foundRoot.Context.TraceID, foundAST.Context.TraceID)
	assert.Equal(t, foundRoot.Context.TraceID, foundDrixy.Context.TraceID)

	// Verify Events and Attributes
	assert.Equal(t, "tenant-matrix-101", foundRoot.Attributes[TenantID])
	assert.Equal(t, 42, foundRoot.Attributes[PullRequestNumber])
	require.Len(t, foundRoot.Events, 1)
	assert.Equal(t, "pr.diff.downloaded", foundRoot.Events[0].Name)

	assert.Equal(t, StatusOK, foundAST.Status.Code)
	assert.Equal(t, StatusError, foundDrixy.Status.Code)
	assert.Contains(t, foundDrixy.Attributes[GenAIRequestModel], "claude-3-7-sonnet")
}

// ============================================================================
// ExecutionTracker Matrix Tests
// ============================================================================

func TestExecutionTrackerMatrix_ComplexAgentLifecycle(t *testing.T) {
	tracker := NewExecutionTracker(500)

	correlationID := uuid.New().String()
	meta := ExecutionCycleMetadata{
		TenantID:  "tenant-obs-501",
		SessionID: "sess-abc-123",
		UserID:    "user-developer",
	}

	t.Run("Standard Successful Cycle with Steps", func(t *testing.T) {
		execID := tracker.StartExecution("DrixyReviewAgent", correlationID, meta, map[string]any{"repo": "scandrix/backend"})
		require.NotEmpty(t, execID)

		tracker.AddStep(execID, StepTypeReasoning, "planner", map[string]any{"plan": "analyze diff chunk 1"}, 12)
		tracker.AddStep(execID, StepTypeLLMCall, "llm-client", map[string]any{"tokens": 450}, 85)
		tracker.AddStep(execID, StepTypeValidation, "ast-validator", map[string]any{"valid": true}, 5)

		tracker.CompleteExecution(execID, map[string]any{"findings_count": 2})

		cycle, exists := tracker.GetExecution(execID)
		require.True(t, exists)
		assert.Equal(t, "completed", cycle.Status)
		assert.Equal(t, "DrixyReviewAgent", cycle.AgentName)
		assert.Equal(t, correlationID, cycle.CorrelationID)
		assert.Equal(t, "tenant-obs-501", cycle.Metadata.TenantID)
		require.NotNil(t, cycle.EndTime)
		assert.True(t, cycle.Duration >= 0)

		// 1 initial start step + 3 custom steps + 1 complete step = 5 steps
		assert.Len(t, cycle.Steps, 5)
		assert.Equal(t, StepTypeStart, cycle.Steps[0].Type)
		assert.Equal(t, StepTypeReasoning, cycle.Steps[1].Type)
		assert.Equal(t, StepTypeLLMCall, cycle.Steps[2].Type)
		assert.Equal(t, StepTypeValidation, cycle.Steps[3].Type)
		assert.Equal(t, StepTypeComplete, cycle.Steps[4].Type)
	})

	t.Run("Failed Execution Cycle", func(t *testing.T) {
		execID := tracker.StartExecution("SandboxRunnerAgent", correlationID, meta, nil)

		tracker.AddStep(execID, StepTypeToolCall, "docker-exec", map[string]any{"cmd": "go test ./..."}, 50)
		tracker.FailExecution(execID, errors.New("timeout waiting for container"))

		cycle, exists := tracker.GetExecution(execID)
		require.True(t, exists)
		assert.Equal(t, "failed", cycle.Status)
		assert.Contains(t, cycle.Error, "timeout waiting for container")
		assert.Equal(t, StepTypeError, cycle.Steps[len(cycle.Steps)-1].Type)
	})
}

func TestExecutionTrackerMatrix_HighConcurrencyStress(t *testing.T) {
	tracker := NewExecutionTracker(1000)
	const numConcurrentCycles = 50
	const stepsPerCycle = 4

	var wg sync.WaitGroup
	wg.Add(numConcurrentCycles)

	for i := 0; i < numConcurrentCycles; i++ {
		go func(idx int) {
			defer wg.Done()
			corrID := fmt.Sprintf("corr-%d", idx)
			meta := ExecutionCycleMetadata{
				TenantID: fmt.Sprintf("tenant-%d", idx%5),
			}

			execID := tracker.StartExecution("ConcurrentReviewAgent", corrID, meta, idx)
			for s := 0; s < stepsPerCycle; s++ {
				tracker.AddStep(execID, StepTypeLLMCall, "llm", map[string]any{"step": s}, 2)
			}
			if idx%5 == 0 {
				tracker.FailExecution(execID, errors.New("deliberate mock failure"))
			} else {
				tracker.CompleteExecution(execID, "done")
			}
		}(i)
	}

	wg.Wait()

	// Verify all executions tracked
	successCount := 0
	failedCount := 0
	for i := 0; i < numConcurrentCycles; i++ {
		corrID := fmt.Sprintf("corr-%d", i)
		found := false
		tracker.mu.RLock()
		for _, c := range tracker.cycles {
			if c.CorrelationID == corrID {
				found = true
				if c.Status == "completed" {
					successCount++
				} else if c.Status == "failed" {
					failedCount++
				}
				break
			}
		}
		tracker.mu.RUnlock()
		assert.True(t, found, "Expected cycle for corrID %s", corrID)
	}

	assert.Equal(t, 10, failedCount)
	assert.Equal(t, 40, successCount)
}

// ============================================================================
// ReviewFlowTracker DAG Matrix Tests
// ============================================================================

func TestReviewFlowTrackerMatrix_MultiStageWorkflow(t *testing.T) {
	tracker := NewReviewFlowTracker()

	reviewID := "rev-matrix-001"
	tenantID := "tenant-alpha"
	wsID := "ws-alpha"
	repoID := "repo-scandrix"
	prNum := 777
	corrID := "corr-rev-777"

	flow := tracker.StartFlow(reviewID, tenantID, wsID, repoID, prNum, corrID)
	require.NotNil(t, flow)

	// Stage 1: Validate payload
	flow.RecordStage(StageValidatePayload, time.Now().Add(-50*time.Millisecond), StatusOK, 100, 20, nil, map[string]any{"rule_count": 5})

	// Stage 2: Fetch Git Diff
	flow.RecordStage(StageFetchGitDiff, time.Now().Add(-30*time.Millisecond), StatusOK, 50, 10, nil, map[string]any{"files_count": 12})

	// Stage 3: Synthesize Review
	flow.RecordStage(StageSynthesizeReview, time.Now().Add(-10*time.Millisecond), StatusOK, 1200, 450, nil, map[string]any{"model": "claude-3-7-sonnet"})

	flow.Complete()

	metrics := tracker.GetAggregateMetrics()
	assert.Equal(t, int64(1), metrics.TotalReviews)
	assert.Equal(t, int64(1), metrics.SuccessfulReviews)
	assert.Equal(t, int64(0), metrics.FailedReviews)
	assert.Equal(t, int64(1830), metrics.TotalTokensSpent)

	assert.True(t, flow.Success)
	assert.Len(t, flow.Stages, 3)
}

// ============================================================================
// Sanitizer + BatchSpanProcessor End-to-End Pipeline Matrix Test
// ============================================================================

func TestOtelSanitizerMatrix_E2EChainProcessor(t *testing.T) {
	memExp := NewMemoryExporter()
	batchProc := NewBatchSpanProcessor(memExp, func(c *BatchProcessorConfig) {
		c.FlushInterval = 20 * time.Millisecond
		c.MaxBatchSize = 10
	})

	sanitizer := NewSanitizationProcessor()
	sanitizer.SetNext(batchProc)

	tracer := NewTracer("scandrix.e2e.sanitized")
	tracer.AddProcessor(sanitizer)

	ctx := context.Background()
	_, span := tracer.Start(ctx, "ExecuteReviewPipeline")

	// Inject sensitive tokens across various attributes
	span.SetAttribute("openai.api_key", "sk-1234567890abcdef1234567890abcdef")
	span.SetAttribute("database.url", "postgres://user:super_secret_password@db.scandrix.internal:5432/core")
	span.SetAttribute("scandrix.cli_key", "scandrix_live_0123456789abcdef0123456789abcdef")
	span.SetAttribute("public.model", "gpt-4o")

	span.AddEvent("agent.prompt.dispatched", nil)

	span.End()

	// Wait for batch flush
	time.Sleep(100 * time.Millisecond)

	spans := memExp.GetSpans()
	require.Len(t, spans, 1)
	exportedSpan := spans[0]

	attrs := exportedSpan.Attributes

	// Sensitive attributes must be redacted
	assert.Equal(t, "[REDACTED]", attrs["openai.api_key"])
	assert.Equal(t, "[REDACTED]", attrs["scandrix.cli_key"])
	assert.Equal(t, "gpt-4o", attrs["public.model"])
}
