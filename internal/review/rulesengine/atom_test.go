// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/rulesengine"
	"github.com/scandrix/backend/pkg/models"
)

func TestRuleAtomCompiler_StructuralDecomposition(t *testing.T) {
	compiler := rulesengine.NewRuleAtomCompiler("scandrix-test-v1")

	rule := &rulesengine.DrixyRule{
		ID:       uuid.MustParse("00000000-0000-0000-0001-000000000010"),
		Slug:     "clean-code-invariants",
		Title:    "Clean Code Invariants",
		Severity: models.SeverityHigh,
		Description: `
### Invariant 1: No debug prints
- MUST NEVER call fmt.Println or console.log in production code.

### Invariant 2: Parameterized SQL
- MUST NOT use db.Query with raw string concatenation or fmt.Sprintf.

### Invariant 3: Documentation
- SHOULD provide comprehensive documentation for exported types.
`,
	}

	examples := []rulesengine.DrixyRuleExample{
		{
			Snippet:     `fmt.Println("debug user info")`,
			IsCorrect:   false,
			Description: "Illegal debug print",
		},
		{
			Snippet:     `logger.Info("user logged in", "user_id", uid)`,
			IsCorrect:   true,
			Description: "Structured logging",
		},
		{
			Snippet:     `db.Query("SELECT * FROM users WHERE id = " + uid)`,
			IsCorrect:   false,
			Description: "SQL concatenation",
		},
		{
			Snippet:     `db.Query("SELECT * FROM users WHERE id = $1", uid)`,
			IsCorrect:   true,
			Description: "Parameterized query",
		},
	}

	atoms := compiler.DecomposeRule(rule, examples)
	require.NotNil(t, atoms)
	assert.Len(t, atoms.Items, 3)

	// Invariant 1 (debug prints) should compile to mechanical T0 detector
	atom1 := atoms.Items[0]
	assert.Equal(t, "clean-code-invariants-atom-1", atom1.ID)
	assert.Equal(t, rulesengine.NormativeNever, atom1.NormativeLevel)
	assert.NotNil(t, atom1.Detector, "expected atom 1 to compile into mechanical detector")
	assert.Empty(t, atom1.DeclineReason)

	// Invariant 2 (SQL concat) should compile to mechanical T0 detector
	atom2 := atoms.Items[1]
	assert.Equal(t, "clean-code-invariants-atom-2", atom2.ID)
	assert.Equal(t, rulesengine.NormativeMustNot, atom2.NormativeLevel)

	// Invariant 3 (documentation) is semantic prose, should stay semantic
	atom3 := atoms.Items[2]
	assert.Equal(t, "clean-code-invariants-atom-3", atom3.ID)
	assert.Equal(t, rulesengine.NormativeShould, atom3.NormativeLevel)
	assert.Nil(t, atom3.Detector)
	assert.Equal(t, "not-mechanical", atom3.DeclineReason)

	// Verify partitioning
	assert.Len(t, atoms.MechanicalAtoms(), 2)
	assert.Len(t, atoms.SemanticAtoms(), 1)

	// Verify freshness tracking
	assert.True(t, atoms.IsFresh(rule.Description, examples))
	assert.False(t, atoms.IsFresh("modified rule description", examples))
}

func TestRuleAtomCompiler_CompileGateReDoSAndPrecision(t *testing.T) {
	compiler := rulesengine.NewRuleAtomCompiler("scandrix-test-v1")

	// 1. ReDoS unsafe pattern rejection
	atom := &rulesengine.DrixyRuleAtom{
		ID:    "test-redos",
		Title: "ReDoS Test",
		Spec:  "Must avoid catastrophic patterns",
	}

	unsafePattern := `(a+)+$`
	res := compiler.EvaluateCompileGate(atom, unsafePattern, "")
	assert.False(t, res.Passed)
	assert.Equal(t, "unsafe-regex", res.DeclineReason)

	// 2. Precision check: rejects detector when it flags good examples
	atomWithExamples := &rulesengine.DrixyRuleAtom{
		ID:    "test-precision",
		Title: "Logger check",
		Spec:  "Use structured logging",
		Examples: []rulesengine.DrixyRuleExample{
			{
				Snippet:   `fmt.Println("debug")`,
				IsCorrect: false,
			},
			{
				Snippet:   `logger.Info("info")`,
				IsCorrect: true,
			},
		},
	}

	// Overly broad pattern matching any word with parenthesis, including logger.Info
	overlyBroad := `\w+\.\w+\(.*\)`
	gateRes := compiler.EvaluateCompileGate(atomWithExamples, overlyBroad, "")
	assert.False(t, gateRes.Passed)
	assert.Equal(t, "flagged-correct-example", gateRes.DeclineReason)

	// 3. Recall check: rejects detector when it fails to match bad examples
	noMatchPattern := `definite_string_that_does_not_occur`
	gateResMissed := compiler.EvaluateCompileGate(atomWithExamples, noMatchPattern, "")
	assert.False(t, gateResMissed.Passed)
	assert.Equal(t, "missed-incorrect-example", gateResMissed.DeclineReason)
}

func TestAtomEvaluator_EvaluateAtoms(t *testing.T) {
	ctx := context.Background()
	evaluator := rulesengine.NewAtomEvaluator()
	reviewID := uuid.New()
	workspaceID := uuid.New()

	detectorRx := `(?i)\b(?:fmt\.Print(?:ln|f)?|console\.log)\s*\(`
	negRx := `(?i)(?:_test\.go)`

	atoms := &rulesengine.DrixyRuleAtoms{
		Items: []*rulesengine.DrixyRuleAtom{
			{
				ID:       "atom-no-debug-prints",
				Title:    "No Debug Prints",
				Spec:     "Do not commit fmt.Println to production files",
				Severity: models.SeverityMedium,
				Category: "style",
				Detector: &rulesengine.CompiledRuleDetector{
					Type:            rulesengine.DetectorRegex,
					Pattern:         detectorRx,
					NegativePattern: negRx,
					Reason:          "Matches unbuffered stdout print",
				},
			},
			{
				ID:       "atom-semantic-only",
				Title:    "Complex Business Invariant",
				Spec:     "Verify idempotent account billing calculations",
				Severity: models.SeverityCritical,
				Category: "business_logic",
			},
		},
	}

	patches := []*diff.FilePatch{
		{
			NewPath: "services/billing/process.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineContext, Content: "func ProcessPayment() {", OldLineNo: 1, NewLineNo: 1},
						{Type: diff.LineAddition, Content: `	fmt.Println("processing payment...")`, NewLineNo: 2},
						{Type: diff.LineAddition, Content: `	return`, NewLineNo: 3},
					},
				},
			},
		},
	}

	report := evaluator.EvaluateAtoms(ctx, reviewID, workspaceID, atoms, patches)
	require.NotNil(t, report)

	assert.Equal(t, 2, report.TotalAtomsEvaluated)
	assert.Equal(t, 1, report.MechanicalViolations)
	assert.Equal(t, 1, report.SemanticPendingAtoms)
	assert.InDelta(t, 0.5, report.ComplianceScore, 0.01)
	require.Len(t, report.Findings, 1)

	finding := report.Findings[0]
	assert.Equal(t, "services/billing/process.go", finding.FilePath)
	assert.Equal(t, 2, finding.StartLine)
	assert.Equal(t, models.SeverityMedium, finding.Severity)
	assert.Equal(t, "No Debug Prints", finding.Title)
	assert.Contains(t, report.ViolatedAtomIDs, "atom-no-debug-prints")
}

func TestInMemoryRuleMetricsStore_TelemetryAndHealthScoring(t *testing.T) {
	ctx := context.Background()
	store := rulesengine.NewInMemoryRuleMetricsStore()

	ruleID := "rule-sec-token-leak"
	slug := "sec-token-leak"

	// Simulate concurrent evaluations
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			matched := idx%2 == 0
			suppressed := idx%4 == 0
			_ = store.RecordEvaluation(ctx, ruleID, slug, matched, suppressed)
		}(i)
	}
	wg.Wait()

	telemetry, err := store.GetRuleTelemetry(ctx, ruleID)
	require.NoError(t, err)
	assert.Equal(t, int64(20), telemetry.Evaluations)
	assert.Equal(t, int64(10), telemetry.Matches)

	// Simulate developer feedback (mostly dismissals / false positives)
	for i := 0; i < 6; i++ {
		_ = store.RecordFeedback(ctx, rulesengine.RuleFeedbackRecord{
			RuleID:   ruleID,
			Feedback: rulesengine.FeedbackNegative,
			Reason:   rulesengine.ReasonFalsePositive,
			Comment:  "Too aggressive on test variables",
		})
	}
	_ = store.RecordFeedback(ctx, rulesengine.RuleFeedbackRecord{
		RuleID:   ruleID,
		Feedback: rulesengine.FeedbackPositive,
		Reason:   rulesengine.ReasonHelpful,
	})

	health, err := store.GetRuleHealth(ctx, ruleID)
	require.NoError(t, err)

	// 1 positive out of 7 feedback items -> Precision ~ 0.14 (< 0.40)
	assert.InDelta(t, 1.0/7.0, health.Precision, 0.05)
	assert.Equal(t, rulesengine.HealthStatusAutoSuppressed, health.Status)
	assert.Contains(t, health.Recommendation, "auto-suppressed")

	lowHealth, err := store.ListLowHealthRules(ctx, 0.50)
	require.NoError(t, err)
	assert.Len(t, lowHealth, 1)
	assert.Equal(t, ruleID, lowHealth[0].RuleID)
}
