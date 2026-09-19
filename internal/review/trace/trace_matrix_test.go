// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package trace

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceMatrix_SpanLifecycleAndParentChildHierarchy(t *testing.T) {
	tracer := NewTracer(500)

	// 1. Root Span: Full PR Review
	rootSpan, rootCtx := tracer.StartSpan(context.Background(), "pipeline_review", "org_enterprise", "repo_core", 404)
	assert.NotEmpty(t, rootSpan.TraceID)
	assert.Empty(t, rootSpan.ParentSpanID)
	assert.Equal(t, "org_enterprise:repo_core:404", rootSpan.SessionID)

	rootSpan.SetAttribute("trigger", "github_webhook")
	rootSpan.RecordEvent("preflight_completed", map[string]interface{}{"files_count": 12})

	// 2. Child Span: Specialist Review Stage
	stageSpan, stageCtx := tracer.StartSpan(rootCtx, "stage_specialist_review", "", "", 0)
	assert.Equal(t, rootSpan.TraceID, stageSpan.TraceID)
	assert.Equal(t, rootSpan.SpanID, stageSpan.ParentSpanID)
	assert.Equal(t, rootSpan.SessionID, stageSpan.SessionID)

	// 3. Grandchild Spans: Individual Specialist Agents
	securitySpan, _ := tracer.StartSpan(stageCtx, "agent_security", "", "", 0)
	assert.Equal(t, rootSpan.TraceID, securitySpan.TraceID)
	assert.Equal(t, stageSpan.SpanID, securitySpan.ParentSpanID)

	securitySpan.AddTokenUsage(1200, 350, 400, 3.0) // $3.00 per million tokens
	securitySpan.End(SpanStatusOK, "")

	perfSpan, _ := tracer.StartSpan(stageCtx, "agent_performance", "", "", 0)
	assert.Equal(t, rootSpan.TraceID, perfSpan.TraceID)
	assert.Equal(t, stageSpan.SpanID, perfSpan.ParentSpanID)

	perfSpan.AddTokenUsage(800, 200, 0, 3.0)
	perfSpan.End(SpanStatusOK, "")

	stageSpan.End(SpanStatusOK, "")
	rootSpan.End(SpanStatusOK, "")

	// Verify span retrieval
	allSpans := tracer.GetSpansByTraceID(rootSpan.TraceID)
	assert.Len(t, allSpans, 4)

	// Verify session aggregation
	usage, cost := tracer.TotalTokenUsageForSession(rootSpan.SessionID)
	assert.Equal(t, 2000, usage.PromptTokens)     // 1200 + 800
	assert.Equal(t, 550, usage.CompletionTokens) // 350 + 200
	assert.Equal(t, 400, usage.CachedTokens)     // 400
	assert.Equal(t, 2550, usage.TotalTokens)
	assert.Greater(t, cost, 0.0)
}

func TestTraceMatrix_DecisionContextPackBudgetAndPinning(t *testing.T) {
	store := NewTraceStore()
	ctx := context.Background()

	orgID := "org_1"
	repoID := "repo_1"

	// 1. Pinned decision: low confidence but PINNED (must never be dropped)
	pinnedDec := &TraceDecision{
		ID:          uuid.New(),
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-001",
		Title:       "Custom Lock-Free Queue",
		Decision:    "Use custom lock-free atomic ring buffer for high-throughput messaging",
		Rationale:   "Standard mutex channel yields 3x latency at 100K msg/sec",
		Type:        DecisionTradeoff,
		Status:      StatusAccepted,
		Origin:      OriginHuman,
		Scope:       []string{"internal/queue/**"},
		Confidence:  0.40,
		Pinned:      true,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.SaveDecision(ctx, pinnedDec))

	// 2. High confidence unpinned decision
	highConfDec := &TraceDecision{
		ID:          uuid.New(),
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-002",
		Title:       "Explicit Transaction Management",
		Decision:    "Always wrap order creation and payment capture in a serializable transaction",
		Rationale:   "Prevents phantom orders during network timeouts",
		Type:        DecisionArchitectural,
		Status:      StatusAccepted,
		Origin:      OriginHuman,
		Scope:       []string{"internal/queue/**", "internal/order/**"},
		Confidence:  0.95,
		Pinned:      false,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.SaveDecision(ctx, highConfDec))

	// 3. Medium confidence unpinned decision
	medConfDec := &TraceDecision{
		ID:          uuid.New(),
		OrgID:       orgID,
		RepoID:      repoID,
		DecisionKey: "ADR-003",
		Title:       "Relaxed Rate Limiting in Staging",
		Decision:    "Permit up to 1000 req/sec in staging environment for load testing",
		Rationale:   "Staging benchmarks must not be throttled",
		Type:        DecisionConvention,
		Status:      StatusAccepted,
		Origin:      OriginHuman,
		Scope:       []string{"internal/queue/**"},
		Confidence:  0.60,
		Pinned:      false,
		CreatedAt:   time.Now().UTC(),
	}
	require.NoError(t, store.SaveDecision(ctx, medConfDec))

	// 4. Query with token budget of 100:
	// Budget accommodates pinned (42 tokens) + highConf (47 tokens) = 89 tokens.
	// medConf (37 tokens) pushes total to 126 > 100, so medConf is dropped!
	pack, err := store.QueryDecisionsForFiles(ctx, orgID, repoID, []string{"internal/queue/ring.go"}, 100)
	require.NoError(t, err)
	require.NotNil(t, pack)

	var keptKeys []string
	for _, d := range pack.Decisions {
		keptKeys = append(keptKeys, d.DecisionKey)
	}

	assert.Contains(t, keptKeys, "ADR-001")
	assert.Contains(t, keptKeys, "ADR-002")
	assert.NotContains(t, keptKeys, "ADR-003")
	assert.Equal(t, 1, pack.DroppedForBudget)

	// Formatted prompt must include the header and decision titles
	prompt := pack.FormatPromptSlice()
	assert.Contains(t, prompt, "Architectural Decisions")
	assert.Contains(t, prompt, "ADR-001")
	assert.Contains(t, prompt, "Lock-Free Queue")
}

func TestTraceMatrix_HighConcurrencyTracingStress(t *testing.T) {
	tracer := NewTracer(5000)
	store := NewTraceStore()

	workers := 20
	iterations := 50

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ctx := context.Background()
				orgID := fmt.Sprintf("org_%d", workerID)
				repoID := fmt.Sprintf("repo_%d", i%5)
				prNum := 1000 + i

				// 1. Trace operations
				rootSpan, rootCtx := tracer.StartSpan(ctx, "concurrent_review", orgID, repoID, prNum)
				rootSpan.RecordEvent("start", map[string]interface{}{"worker": workerID})

				childSpan, _ := tracer.StartSpan(rootCtx, "agent_task", "", "", 0)
				childSpan.AddTokenUsage(100, 50, 20, 2.5)
				childSpan.End(SpanStatusOK, "")

				rootSpan.End(SpanStatusOK, "")

				// 2. Store operations
				dec := &TraceDecision{
					ID:          uuid.New(),
					OrgID:       orgID,
					RepoID:      repoID,
					DecisionKey: fmt.Sprintf("DEC_%d_%d", workerID, i),
					Title:       fmt.Sprintf("Concurrent Decision %d", i),
					Decision:    "Concurrent decision content",
					Type:        DecisionConvention,
					Status:      StatusAccepted,
					Origin:      OriginAgent,
					Scope:       []string{fmt.Sprintf("pkg/mod_%d/**", workerID)},
					Confidence:  0.85,
					CreatedAt:   time.Now().UTC(),
				}
				_ = store.SaveDecision(ctx, dec)
				_, _ = store.QueryDecisionsForFiles(ctx, orgID, repoID, []string{fmt.Sprintf("pkg/mod_%d/file.go", workerID)}, 500)
			}
		}(w)
	}

	wg.Wait()
}
