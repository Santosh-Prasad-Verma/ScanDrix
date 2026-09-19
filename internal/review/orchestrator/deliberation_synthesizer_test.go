// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockArbiter struct {
	evaluations []ArbiterEvaluation
	err         error
}

func (m *mockArbiter) EvaluateFindings(ctx context.Context, findings []AgentFinding, changedFiles []ChangedFile) ([]ArbiterEvaluation, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.evaluations, nil
}

func TestDeliberationSynthesizer_SynthesizeReview(t *testing.T) {
	finding1ID := uuid.New()
	finding2ID := uuid.New()
	finding3ID := uuid.New()

	finding1 := AgentFinding{
		ID:          finding1ID,
		AgentName:   "security",
		FilePath:    "auth.go",
		StartLine:   10,
		EndLine:     12,
		Severity:    models.SeverityCritical,
		Title:       "Real vulnerability",
		Description: "SQL injection vulnerability detected.",
		Blocking:    true,
	}

	finding2 := AgentFinding{
		ID:          finding2ID,
		AgentName:   "bug",
		FilePath:    "auth.go",
		StartLine:   20,
		EndLine:     22,
		Severity:    models.SeverityHigh,
		Title:       "False positive nil check",
		Description: "Unchecked dereference.",
		Blocking:    true,
	}

	finding3 := AgentFinding{
		ID:          finding3ID,
		AgentName:   "performance",
		FilePath:    "auth.go",
		StartLine:   30,
		EndLine:     35,
		Severity:    models.SeverityLow,
		Title:       "Small allocation optimization",
		Description: "Repeated string formatting in loop.",
		Blocking:    false,
	}

	mockArb := &mockArbiter{
		evaluations: []ArbiterEvaluation{
			{
				FindingID: finding1ID.String(),
				Verdict:   ArbiterConfirmed,
			},
			{
				FindingID: finding2ID.String(),
				Verdict:   ArbiterDisputed, // False positive eliminated!
				Reasoning: "Validated by framework guard in middleware",
			},
			{
				FindingID:        finding3ID.String(),
				Verdict:          ArbiterDowngraded,
				AdjustedSeverity: models.SeverityInfo,
			},
		},
	}

	synth := NewDeliberationSynthesizer(mockArb, nil, nil)

	input := ReviewAgentInput{
		PRNumber: 42,
		Title:    "Add user authentication",
		ChangedFiles: []ChangedFile{
			{
				Filename: "auth.go",
				Content:  "package auth\n...", // No patch constraint so full file accepted
			},
		},
		ReviewOptions: DefaultReviewOptions(),
	}

	outputs := []ReviewAgentOutput{
		{
			AgentName: "security",
			Findings:  []AgentFinding{finding1},
		},
		{
			AgentName: "bug",
			Findings:  []AgentFinding{finding2},
		},
		{
			AgentName: "performance",
			Findings:  []AgentFinding{finding3},
		},
	}

	res, err := synth.SynthesizeReview(context.Background(), input, outputs)
	require.NoError(t, err)
	require.NotNil(t, res)

	// finding2 should be eliminated by Arbiter
	assert.Len(t, res.Findings, 2)
	assert.Equal(t, 1, res.DeliberationMetadata.FindingsRemovedByArbiter)
	assert.Equal(t, 1, res.DeliberationMetadata.FindingsDowngraded)

	// Verdict should be REQUEST_CHANGES because finding1 is Critical & Blocking
	assert.Equal(t, VerdictRequestChanges, res.Verdict)

	// Top finding must be the blocking Critical finding
	assert.Equal(t, finding1ID, res.Findings[0].ID)
	assert.Equal(t, models.SeverityCritical, res.Findings[0].Severity)
	assert.True(t, res.Findings[0].Blocking)

	// Second finding should be downgraded to INFO
	assert.Equal(t, models.SeverityInfo, res.Findings[1].Severity)

	// Check that summary contains the expected header
	assert.Contains(t, res.Summary, "ScanDrix AI Review: CHANGES REQUESTED")
	assert.Contains(t, res.Summary, "Real vulnerability")
}

func TestComputeFindingFingerprint(t *testing.T) {
	fp1 := ComputeFindingFingerprint("pkg/auth/login.go", 12, 14, "Security", "SQL Injection in handler")
	fp2 := ComputeFindingFingerprint("pkg/auth/login.go", 11, 15, "security", "sql injection in handler")
	// Same 5-line window and normalized title -> identical fingerprint
	assert.Equal(t, fp1, fp2)

	fp3 := ComputeFindingFingerprint("pkg/auth/login.go", 90, 95, "security", "sql injection in handler")
	assert.NotEqual(t, fp1, fp3)
}

func TestSpecialistDomainAuthority(t *testing.T) {
	assert.InDelta(t, 1.6, SpecialistDomainAuthority("security", "vulnerability"), 0.01)
	assert.InDelta(t, 1.6, SpecialistDomainAuthority("drixy_rules", "custom_rule"), 0.01)
	assert.InDelta(t, 1.5, SpecialistDomainAuthority("performance", "database_n_plus_one"), 0.01)
	assert.InDelta(t, 1.5, SpecialistDomainAuthority("bug", "logic_error"), 0.01)
	assert.InDelta(t, 1.4, SpecialistDomainAuthority("architecture", "coupling"), 0.01)
	assert.InDelta(t, 1.0, SpecialistDomainAuthority("generalist", "anything"), 0.01)
}

func TestReconcileFindings_MultiAgentCollision(t *testing.T) {
	findingGeneralist := AgentFinding{
		ID:          uuid.New(),
		AgentName:   "generalist",
		FilePath:    "services/payment.go",
		StartLine:   45,
		EndLine:     48,
		Severity:    models.SeverityMedium,
		Confidence:  "MEDIUM",
		Category:    "bug",
		Title:       "Potential concurrency issue",
		Description: "Shared map access without lock.",
		Blocking:    false,
	}

	findingBug := AgentFinding{
		ID:            uuid.New(),
		AgentName:     "bug",
		FilePath:      "services/payment.go",
		StartLine:     46,
		EndLine:       49,
		Severity:      models.SeverityCritical,
		Confidence:    "HIGH",
		Category:      "race_condition",
		Title:         "Data race on payment cache",
		Description:   "Concurrent read/write on un-synchronized map causes crash.",
		SuggestedDiff: "@@ -46,3 +46,5 @@\n+mu.Lock()\n+defer mu.Unlock()\n",
		Blocking:      true,
	}

	reconciled := ReconcileFindings([]AgentFinding{findingGeneralist, findingBug})
	require.Len(t, reconciled, 1)

	merged := reconciled[0]
	assert.Equal(t, models.SeverityCritical, merged.Severity)
	assert.True(t, merged.Blocking)
	assert.Equal(t, "HIGH", merged.Confidence)
	assert.ElementsMatch(t, []string{"bug", "generalist"}, merged.ContributingAgents)
	assert.NotEmpty(t, merged.SuggestedDiff)
	assert.Equal(t, "RESOLVED", merged.DisputeStatus)
}

func TestHeuristicSkepticalArbiter_SpeculativeDowngrade(t *testing.T) {
	arbiter := NewHeuristicSkepticalArbiter()

	finding := AgentFinding{
		ID:          uuid.New(),
		FilePath:    "config.go",
		StartLine:   10,
		EndLine:     12,
		Severity:    models.SeverityCritical,
		Blocking:    true,
		Title:       "Might want to consider refactoring config loader",
		Description: "It could potentially be cleaner if you used a builder pattern.",
	}

	files := []ChangedFile{{Filename: "config.go", Content: "package config"}}
	evals, err := arbiter.EvaluateFindings(context.Background(), []AgentFinding{finding}, files)
	require.NoError(t, err)
	require.Len(t, evals, 1)

	assert.Equal(t, ArbiterDowngraded, evals[0].Verdict)
	assert.Equal(t, models.SeverityLow, evals[0].AdjustedSeverity)
	require.NotNil(t, evals[0].AdjustedBlocking)
	assert.False(t, *evals[0].AdjustedBlocking)
}

func TestHeuristicSkepticalArbiter_SqlInjectionRefutation(t *testing.T) {
	arbiter := NewHeuristicSkepticalArbiter()

	finding := AgentFinding{
		ID:           uuid.New(),
		FilePath:     "db/user.go",
		StartLine:    25,
		EndLine:      27,
		Severity:     models.SeverityCritical,
		Category:     "security",
		Title:        "SQL Injection vulnerability",
		Description:  "Dynamic query detected.",
		ExistingCode: `db.Query("SELECT id FROM users WHERE email = $1", email)`,
	}

	files := []ChangedFile{{Filename: "db/user.go", Content: "code"}}
	evals, err := arbiter.EvaluateFindings(context.Background(), []AgentFinding{finding}, files)
	require.NoError(t, err)
	require.Len(t, evals, 1)

	assert.Equal(t, ArbiterDisputed, evals[0].Verdict)
	assert.Contains(t, evals[0].Reasoning, "parameterized query placeholders")
}

func TestHeuristicSkepticalArbiter_ConsensusEnhancement(t *testing.T) {
	arbiter := NewHeuristicSkepticalArbiter()

	finding := AgentFinding{
		ID:                 uuid.New(),
		FilePath:           "api/auth.go",
		StartLine:          15,
		EndLine:            18,
		Severity:           models.SeverityMedium,
		Category:           "security",
		Title:              "Missing authorization header validation",
		Description:        "Requests can pass without bearer token check.",
		ContributingAgents: []string{"bug", "security"},
	}

	files := []ChangedFile{{Filename: "api/auth.go", Content: "code"}}
	evals, err := arbiter.EvaluateFindings(context.Background(), []AgentFinding{finding}, files)
	require.NoError(t, err)
	require.Len(t, evals, 1)

	assert.Equal(t, ArbiterEnhanced, evals[0].Verdict)
	require.NotNil(t, evals[0].AdjustedBlocking)
	assert.True(t, *evals[0].AdjustedBlocking)
}

func TestHeuristicSkepticalArbiter_HallucinatedFileDisputed(t *testing.T) {
	arbiter := NewHeuristicSkepticalArbiter()

	finding := AgentFinding{
		ID:          uuid.New(),
		FilePath:    "non_existent_file.go",
		StartLine:   10,
		EndLine:     15,
		Severity:    models.SeverityHigh,
		Title:       "Memory leak in worker pool",
		Description: "Goroutine leak detected.",
	}

	files := []ChangedFile{{Filename: "real_file.go", Content: "code"}}
	evals, err := arbiter.EvaluateFindings(context.Background(), []AgentFinding{finding}, files)
	require.NoError(t, err)
	require.Len(t, evals, 1)

	assert.Equal(t, ArbiterDisputed, evals[0].Verdict)
	assert.Contains(t, evals[0].Reasoning, "not found in PR changed files")
}

func TestHeuristicSkepticalArbiter_CustomRuleConfirmed(t *testing.T) {
	arbiter := NewHeuristicSkepticalArbiter()
	ruleID := uuid.New()

	finding := AgentFinding{
		ID:          uuid.New(),
		RuleID:      &ruleID,
		FilePath:    "models/user.go",
		StartLine:   5,
		EndLine:     8,
		Severity:    models.SeverityCritical,
		Title:       "Violates Custom Rule: No Plaintext Passwords",
		Description: "Rule explicitly flags plain password fields.",
	}

	files := []ChangedFile{{Filename: "models/user.go", Content: "code"}}
	evals, err := arbiter.EvaluateFindings(context.Background(), []AgentFinding{finding}, files)
	require.NoError(t, err)
	require.Len(t, evals, 1)

	assert.Equal(t, ArbiterConfirmed, evals[0].Verdict)
	assert.Contains(t, evals[0].Reasoning, "Validated by custom Drixy rule definition")
}
