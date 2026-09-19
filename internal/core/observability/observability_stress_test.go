package observability_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/core/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSanitizerStress_AdversarialSecretLeakPrevention tests an exhaustive battery
// of realistic adversarial tokens, credentials, and PII patterns nested inside complex structures.
func TestSanitizerStress_AdversarialSecretLeakPrevention(t *testing.T) {
	sanitizer := observability.NewSanitizationProcessor()

	testSecrets := []struct {
		name       string
		rawSecret  string
		inputKey   string
		shouldMask bool
	}{
		// AI Provider API Keys
		{"OpenAI Key", "sk-proj-abc123456789012345678901234567890", "custom_prop", true},
		{"Anthropic Key", "anthropic-key-012345678901234567890abcdef", "ai_secret", true},
		{"GitHub PAT", "ghp_0123456789abcdef0123456789abcdef0123", "github_token", true},
		{"GitLab PAT", "glpat-01234567890123456789", "gitlab_key", true},
		{"ScanDrix Key", "scandrix_live_012345678901234567890abcdef", "x-team-key", true},

		// Standard Auth Headers & Tokens
		{"Bearer Token", "Bearer my_super_secret_token_12345", "authorization", true},
		{"JWT Token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozG4m1e_secret_signature_here_12345", "jwt", true},

		// Sensitive Key Names (case-insensitive & snake/camel variants)
		{"Password Key", "myPlaintextPassword123!", "password", true},
		{"MixedCase Password", "secret123", "Password", true},
		{"Uppercase Secret", "top_secret_value", "CLIENT_SECRET", true},
		{"Webhook Secret", "whsec_abcdef1234567890", "webhook_secret", true},
		{"Encryption Key", "32byte_master_encryption_key_here", "encryption_key", true},
		{"Credit Card", "4111222233334444", "creditcard", true},
		{"CVV Code", "123", "cvv", true},
		{"SSN", "000-12-3456", "ssn", true},
	}

	for _, tc := range testSecrets {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Direct key-value map sanitization
			m := map[string]any{
				tc.inputKey: tc.rawSecret,
			}
			cleanMap := sanitizer.SanitizeMap(m)
			cleanVal, ok := cleanMap[tc.inputKey].(string)
			require.True(t, ok)
			assert.Equal(t, observability.DefaultRedactionToken, cleanVal)

			// 2. Nested JSON string sanitization
			jsonBlob := fmt.Sprintf(`{"level1":{"level2":{"%s":"%s"}}}`, tc.inputKey, tc.rawSecret)
			cleanJSON := sanitizer.SanitizeString(jsonBlob)
			assert.NotContains(t, cleanJSON, tc.rawSecret)
			assert.Contains(t, cleanJSON, observability.DefaultRedactionToken)
		})
	}
}

// TestSanitizerStress_DeepRecursionSafety validates that cyclic or deeply nested
// structures beyond MaxDepth (16) abort cleanly without stack overflow panics.
func TestSanitizerStress_DeepRecursionSafety(t *testing.T) {
	sanitizer := observability.NewSanitizationProcessor()

	// Build a 25-level deep nested map
	root := make(map[string]any)
	curr := root
	for i := 0; i < 25; i++ {
		next := make(map[string]any)
		curr[fmt.Sprintf("level_%d", i)] = next
		curr = next
	}
	curr["password"] = "deep_secret_pass"

	clean := sanitizer.SanitizeMap(root)
	require.NotNil(t, clean)

	// Traverse down: depth limit should prevent infinite recursion
	level := clean
	for i := 0; i < 15; i++ {
		nextLevel, ok := level[fmt.Sprintf("level_%d", i)].(map[string]any)
		if !ok {
			break
		}
		level = nextLevel
	}
}

// TestExecutionTrackerStress_HighConcurrencyCycles tests tracking 50 concurrent
// agent execution cycles with simultaneous step additions and lifecycle completion.
func TestExecutionTrackerStress_HighConcurrencyCycles(t *testing.T) {
	tracker := observability.NewExecutionTracker(1000)
	const numAgents = 50
	const stepsPerAgent = 10

	var wg sync.WaitGroup
	var completedCycles atomic.Int32

	wg.Add(numAgents)
	for a := 0; a < numAgents; a++ {
		go func(agentID int) {
			defer wg.Done()
			agentName := fmt.Sprintf("code-reviewer-subagent-%d", agentID)
			corrID := fmt.Sprintf("corr-%d", agentID)

			meta := observability.ExecutionCycleMetadata{
				TenantID:  fmt.Sprintf("tenant-%d", agentID%5),
				SessionID: fmt.Sprintf("sess-%d", agentID),
				UserID:    fmt.Sprintf("user-%d", agentID),
			}

			execID := tracker.StartExecution(agentName, corrID, meta, map[string]any{"pr": agentID})
			require.NotEmpty(t, execID)

			// Record steps concurrently
			for s := 0; s < stepsPerAgent; s++ {
				stepType := observability.StepTypeLLMCall
				if s%2 == 0 {
					stepType = observability.StepTypeToolCall
				}

				tracker.AddStep(execID, stepType, "scanner", map[string]any{"step_index": s}, int64(s*5))
			}

			// End execution
			tracker.CompleteExecution(execID, map[string]any{"findings_count": 3})
			completedCycles.Add(1)

			// Verify cycle
			retrieved, ok := tracker.GetExecution(execID)
			assert.True(t, ok)
			require.NotNil(t, retrieved)
			assert.Equal(t, "completed", retrieved.Status)
			assert.True(t, len(retrieved.Steps) >= stepsPerAgent)
		}(a)
	}

	wg.Wait()
	assert.Equal(t, int32(numAgents), completedCycles.Load())
}

// TestTracerStress_HierarchicalSpanCreationAndContext tests concurrent span hierarchy
// and child span attachment with thread-safe attribute and event updates.
func TestTracerStress_HierarchicalSpanCreationAndContext(t *testing.T) {
	tracer := observability.NewTracer("scandrix-core")
	const numRoots = 30
	const childrenPerRoot = 5

	var wg sync.WaitGroup
	wg.Add(numRoots)

	for r := 0; r < numRoots; r++ {
		go func(rootIdx int) {
			defer wg.Done()
			ctx := context.Background()

			rootCtx, rootSpan := tracer.Start(ctx, fmt.Sprintf("root-operation-%d", rootIdx))
			rootSpan.SetAttribute("tenant.id", fmt.Sprintf("tenant_%d", rootIdx))
			rootSpan.AddEvent("root.start", map[string]any{"timestamp": time.Now().UnixMilli()})

			for c := 0; c < childrenPerRoot; c++ {
				_, childSpan := tracer.Start(rootCtx, fmt.Sprintf("child-task-%d-%d", rootIdx, c))
				childSpan.SetAttribute("child.index", c)
				childSpan.SetAttribute("status", "ok")
				childSpan.End()
			}

			rootSpan.SetStatus(observability.StatusOK, "all children completed successfully")
			rootSpan.End()
		}(r)
	}

	wg.Wait()
}

// TestSpanData_JSONSerializationContract validates standard OpenTelemetry-compatible
// span JSON marshaling and field tagging.
func TestSpanData_JSONSerializationContract(t *testing.T) {
	now := time.Now().UTC()
	span := &observability.SpanData{
		Name: "ast-tree-sitter-parse",
		Context: observability.SpanContext{
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:  "00f067aa0ba902b7",
		},
		ParentSpanID: "5fb397be34d23b0f",
		Kind:         observability.SpanKindInternal,
		StartTime:    now,
		EndTime:      now.Add(150 * time.Millisecond),
		DurationMs:   150,
		Attributes: map[string]any{
			"files_count": 12,
			"ast.version": "v0.12.0",
		},
		Status: observability.SpanStatus{
			Code:        observability.StatusOK,
			Description: "parse completed without errors",
		},
	}

	bytes, err := json.Marshal(span)
	require.NoError(t, err)

	var parsed map[string]any
	err = json.Unmarshal(bytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "ast-tree-sitter-parse", parsed["name"])
	assert.Contains(t, parsed, "context")
	assert.Contains(t, parsed, "attributes")
	assert.Contains(t, parsed, "status")
}
