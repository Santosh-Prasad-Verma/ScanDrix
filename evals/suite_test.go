// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package evals_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/evals"
	"github.com/scandrix/backend/evals/drixyrules"
	"github.com/scandrix/backend/evals/investigation"
	"github.com/scandrix/backend/evals/parser"
	"github.com/scandrix/backend/evals/prsummary"
	"github.com/scandrix/backend/evals/promotion"
	"github.com/scandrix/backend/evals/results"
	"github.com/scandrix/backend/evals/scorer"
	"github.com/scandrix/backend/evals/secondary"
	"github.com/scandrix/backend/evals/structuredoutputs"
	"github.com/scandrix/backend/evals/tracecontext"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/review/agentcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineGate_PreflightAllDomains(t *testing.T) {
	report := evals.RunPreflight(evals.ProfileHarness)
	assert.True(t, report.Passed, "Preflight must pass across all 11 evaluation domains")
	assert.Empty(t, report.FatalFailures)
	assert.NotEmpty(t, report.Checks)

	err := evals.ExecuteEngineGate(context.Background(), evals.ProfileHarness)
	require.NoError(t, err)
}

func TestInvestigationEvaluator(t *testing.T) {
	state := &contracts.RunState{
		Steps: []contracts.RunStep{
			{
				Message: contracts.AgentMessage{
					Role: contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{
						{
							Name: "readFile",
							Input: map[string]any{
								"path": "internal/auth/token.go",
							},
						},
						{
							Name: "grep",
							Input: map[string]any{
								"query": "jwt.Parse",
							},
						},
					},
				},
			},
		},
	}

	expectations := []investigation.ToolCallExpectation{
		{
			Tool:         "readFile",
			PathEndsWith: "token.go",
		},
		{
			Tool:    "grep",
			Pattern: "jwt.Parse",
		},
	}

	res := investigation.EvaluateInvestigation(state, expectations, 2)
	assert.True(t, res.Passed)
	assert.Equal(t, 2, res.TotalToolCalls)
	assert.Equal(t, 2, res.MatchedCalls)
	assert.Equal(t, 1.0, res.Recall)
}

func TestDrixyRulesBehavioralScoring(t *testing.T) {
	groundTruth := []drixyrules.Site{
		{File: "service.go", Line: 45},
		{File: "handler.go", Line: 100},
	}

	// Model flags line 46 on service.go (diff = 1 <= tolerance 2)
	// and line 200 on other.go (false positive)
	flags := []drixyrules.Site{
		{File: "service.go", Line: 46},
		{File: "other.go", Line: 200},
	}

	score := drixyrules.ScoreCase(groundTruth, flags, 2)
	assert.Equal(t, 1, score.Caught)
	assert.Equal(t, 1, score.OnTarget)
	assert.Equal(t, 0.5, score.Recall)
	assert.Equal(t, 0.5, score.Precision)
	assert.Equal(t, 0.5, score.F1Score)

	// Test ParseViolations from markdown fence
	markdown := "Here are the violations:\n```json\n{\"violations\": [{\"file\": \"api.go\", \"line\": 50}]}\n```"
	parsed := drixyrules.ParseViolations(markdown)
	require.Len(t, parsed, 1)
	assert.Equal(t, "api.go", parsed[0].File)
	assert.Equal(t, 50, parsed[0].Line)
}

func TestDiffParserEvaluator(t *testing.T) {
	rawDiff := `diff --git a/pkg/crypto/aes.go b/pkg/crypto/aes.go
--- a/pkg/crypto/aes.go
+++ b/pkg/crypto/aes.go
@@ -10,4 +10,6 @@ func Encrypt() {
+	cipher.NewGCM()
+	return nil
 }`

	files, err := parser.ParseUnifiedDiff(rawDiff)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "pkg/crypto/aes.go", files[0].NewPath)
	require.Len(t, files[0].Hunks, 1)
	assert.Equal(t, 10, files[0].Hunks[0].NewStart)
	assert.Equal(t, 6, files[0].Hunks[0].NewLines)

	passed, msg := parser.EvaluateParser(rawDiff, 1, 1)
	assert.True(t, passed, msg)
}

func TestPRSummaryEvaluator(t *testing.T) {
	goodSummary := `# PR Summary: Add KMS Envelope Encryption
## Walkthrough
This PR refactors our KMS package to use envelope encryption with local DEKs.
### Changed Files
- pkg/crypto/envelope.go
- pkg/crypto/kms.go
### Risk Assessment
Breaking change: key rotation now requires valid KMS provider permissions.`

	res := prsummary.EvaluatePRSummary(goodSummary)
	assert.True(t, res.Passed)
	assert.True(t, res.HasHeadline)
	assert.True(t, res.HasWalkthrough)
	assert.True(t, res.HasChangedFiles)
	assert.True(t, res.HasRiskAssessment)
	assert.GreaterOrEqual(t, res.Score, 0.8)
}

func TestPromotionEvaluator(t *testing.T) {
	validFinding := agentcore.FinderSuggestion{
		RelevantFile:      "controller.go",
		ExistingCode:      "userId := req.Param(\"id\")",
		ImprovedCode:      "userId := session.GetUserId()",
		Confidence:        0.90,
		SuggestionContent: "IDOR vulnerability: using unauthenticated request parameter",
	}

	res := promotion.EvaluatePromotion(validFinding, 0.70)
	assert.True(t, res.Promoted)

	// Low confidence finding should be suppressed
	lowConfFinding := validFinding
	lowConfFinding.Confidence = 0.50
	resLow := promotion.EvaluatePromotion(lowConfFinding, 0.70)
	assert.False(t, resLow.Promoted)
	assert.Contains(t, resLow.Reason, "Confidence below promotion floor")
}

func TestResultsRecorderAndAggregator(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "eval_results.jsonl")

	recorder := results.NewResultsRecorder(filePath)

	err := recorder.Record(results.EvalRunRecord{
		Suite:    "severity",
		Model:    "claude-sonnet-4.5",
		Provider: "anthropic",
		Passed:   true,
		Score:    0.95,
	})
	require.NoError(t, err)

	err = recorder.Record(results.EvalRunRecord{
		Suite:    "severity",
		Model:    "claude-sonnet-4.5",
		Provider: "anthropic",
		Passed:   false,
		Score:    0.60,
	})
	require.NoError(t, err)

	stats, err := recorder.Aggregate()
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalRuns)
	assert.Equal(t, 1, stats.PassedRuns)
	assert.Equal(t, 0.5, stats.PassRate)
	assert.InDelta(t, 0.775, stats.MeanScore, 0.001)
}

func TestScorerCompositeScore(t *testing.T) {
	score := scorer.ComputeCompositeScore(1.0, 1.0, 1.0, 1.0)
	assert.True(t, score.Passed)
	assert.Equal(t, 1.0, score.TotalScore)

	failingScore := scorer.ComputeCompositeScore(0.3, 0.2, 0.5, 0.4)
	assert.False(t, failingScore.Passed)
}

func TestSecondaryVulnerabilityEvaluator(t *testing.T) {
	cases := secondary.BuiltInSecondaryTestCases()
	require.NotEmpty(t, cases)

	finding := agentcore.FinderSuggestion{
		RelevantFile:      "internal/worker/pool.go",
		SuggestionContent: "Detected async foreach callback that triggers unhandled goroutine race",
		ImprovedCode:      "Use for _, item := range items with sync.WaitGroup",
	}

	matched := secondary.EvaluateSecondaryPass([]agentcore.FinderSuggestion{finding}, cases[2])
	assert.True(t, matched)
}

func TestStructuredOutputsExtraction(t *testing.T) {
	type TestReviewPayload struct {
		Summary     string `json:"summary"`
		TotalIssues int    `json:"total_issues"`
	}

	rawText := "Here is my output:\n```json\n{\n  \"summary\": \"Looks good\",\n  \"total_issues\": 0\n}\n```\nHope that helps!"
	extracted, err := structuredoutputs.EvaluateStructuredOutputExtraction[TestReviewPayload](rawText)
	require.NoError(t, err)
	assert.Equal(t, "Looks good", extracted.Summary)
	assert.Equal(t, 0, extracted.TotalIssues)
}

func TestTraceContextEfficiency(t *testing.T) {
	state := &contracts.RunState{
		Status: contracts.StatusCompleted,
		Steps: []contracts.RunStep{
			{
				Message: contracts.AgentMessage{
					Role:      contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{{Name: "readFile"}},
				},
			},
			{
				Message: contracts.AgentMessage{
					Role:      contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{{Name: "submitResult"}},
				},
			},
		},
		Usage: contracts.TokenUsage{
			InputTokens:  1200,
			OutputTokens: 300,
		},
		Trace: []contracts.TraceEvent{
			{Kind: "compression", Source: "compression"},
		},
	}

	res := tracecontext.EvaluateTraceEfficiency(state, 5000)
	assert.True(t, res.Passed)
	assert.Equal(t, 2, res.TotalSteps)
	assert.Equal(t, 1500, res.TotalTokens)
	assert.Equal(t, 750.0, res.AverageTokensPerStep)
	assert.Equal(t, 1, res.CompressionEvents)
}
