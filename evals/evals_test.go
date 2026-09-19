// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package evals_test

import (
	"context"
	"testing"

	"github.com/scandrix/backend/evals"
	"github.com/scandrix/backend/evals/anchoring"
	"github.com/scandrix/backend/evals/dedup"
	"github.com/scandrix/backend/evals/format"
	"github.com/scandrix/backend/evals/severity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineGatePreflight(t *testing.T) {
	report := evals.RunPreflight(evals.ProfileHarness)
	assert.True(t, report.Passed)
	assert.Empty(t, report.FatalFailures)
	assert.GreaterOrEqual(t, len(report.Checks), 4)

	err := evals.ExecuteEngineGate(context.Background(), evals.ProfileHarness)
	require.NoError(t, err)
}

func TestAnchoringEval(t *testing.T) {
	hunks := []anchoring.DiffHunk{
		{
			FilePath:  "internal/auth/password.go",
			NewStart:  20,
			NewLength: 30,
		},
	}

	// Line 25 is inside [20, 50]
	resValid := anchoring.EvaluateAnchor(anchoring.CommentAnchor{
		FilePath:  "internal/auth/password.go",
		LineStart: 25,
	}, hunks)
	assert.True(t, resValid.Valid)
	assert.True(t, resValid.IsInsideHunk)

	// Line 75 is outside [20, 50]
	resOut := anchoring.EvaluateAnchor(anchoring.CommentAnchor{
		FilePath:  "internal/auth/password.go",
		LineStart: 75,
	}, hunks)
	assert.False(t, resOut.Valid)
	assert.False(t, resOut.IsInsideHunk)

	// Unchanged file
	resMissing := anchoring.EvaluateAnchor(anchoring.CommentAnchor{
		FilePath:  "cmd/api/main.go",
		LineStart: 10,
	}, hunks)
	assert.False(t, resMissing.Valid)
}

func TestDedupEval(t *testing.T) {
	findings := []dedup.CandidateFinding{
		{
			ID:                "f1",
			FilePath:          "pkg/util.go",
			LineStart:         15,
			SuggestionContent: "Missing error check when opening file descriptor in utility function.",
		},
		{
			ID:                "f2",
			FilePath:          "pkg/util.go",
			LineStart:         16,
			SuggestionContent: "Check error returned when opening the file descriptor in utility helper.",
		},
		{
			ID:                "f3",
			FilePath:          "pkg/other.go",
			LineStart:         15,
			SuggestionContent: "Different file entirely with unrelated logic.",
		},
	}

	collapsed := dedup.CollapseNearDuplicates(findings)
	assert.Len(t, collapsed, 2, "near duplicates on the same file/line must collapse")
	assert.Equal(t, "f1", collapsed[0].ID)
	assert.Equal(t, "f3", collapsed[1].ID)
}

func TestSeverityEval(t *testing.T) {
	exact := severity.EvaluateSeverity("CRITICAL", "critical")
	assert.True(t, exact.IsCorrect)
	assert.Equal(t, 0, exact.WeightDelta)

	near := severity.EvaluateSeverity("high", "critical")
	assert.False(t, near.IsCorrect)
	assert.True(t, near.IsWithinOne)
	assert.Equal(t, 1, near.WeightDelta)

	far := severity.EvaluateSeverity("low", "critical")
	assert.False(t, far.IsWithinOne)
	assert.Equal(t, 3, far.WeightDelta)
}

func TestFormatEval(t *testing.T) {
	// Raw JSON
	res1 := format.EvaluateStructuredFormat(`{"status": "ok"}`)
	assert.True(t, res1.ValidJSON)
	assert.False(t, res1.RequiredRepair)

	// Markdown fenced JSON
	res2 := format.EvaluateStructuredFormat("```json\n{\"status\": \"repaired\"}\n```")
	assert.True(t, res2.ValidJSON)
	assert.True(t, res2.RequiredRepair)
	assert.True(t, res2.HasMarkdownWrap)

	// Invalid text
	res3 := format.EvaluateStructuredFormat("Just plain text with no json")
	assert.False(t, res3.ValidJSON)
}
