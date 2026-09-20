package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepConfigHierarchyService(t *testing.T) {
	service := NewDeepConfigHierarchyService(nil)
	ctx := context.Background()

	orgID := "org-acme"
	teamID := "team-backend"
	repoID := "repo-payments"

	t.Run("Resolves System Defaults when no overrides exist", func(t *testing.T) {
		cfg, err := service.ResolveEffectiveConfig(ctx, "org-new", "", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, "claude-3-7-sonnet", cfg.ModelSlots[SlotPrimaryTriage].ModelID)
		assert.Equal(t, "continuous", cfg.Cadence)
		assert.Equal(t, 250_000, cfg.MaxTokensPerReview)
	})

	t.Run("Organization Override Cascades to Downstream Scopes", func(t *testing.T) {
		orgCfg := EnterpriseCodeReviewConfig{
			Tier:           TierOrganization,
			OrganizationID: orgID,
			BaselineConfig: domain.CodeReviewConfig{
				Strictness: domain.StrictnessStrict,
			},
			Cadence: "nightly",
		}
		err := service.SaveConfig(ctx, orgCfg)
		require.NoError(t, err)

		effective, err := service.ResolveEffectiveConfig(ctx, orgID, teamID, repoID, "", "")
		require.NoError(t, err)
		assert.Equal(t, domain.StrictnessStrict, effective.BaselineConfig.Strictness)
		assert.Equal(t, "nightly", effective.Cadence)
	})

	t.Run("Directory Glob Pattern Overrides for Specific File Paths", func(t *testing.T) {
		dirCfg := EnterpriseCodeReviewConfig{
			Tier:           TierDirectory,
			OrganizationID: orgID,
			PathPattern:    "sensitive/*.go",
			BaselineConfig: domain.CodeReviewConfig{
				MaxSuggestions: 50,
			},
			ConcurrencyLimit: 10,
		}
		err := service.SaveConfig(ctx, dirCfg)
		require.NoError(t, err)

		// File matching pattern
		matchCfg, err := service.ResolveEffectiveConfig(ctx, orgID, teamID, repoID, "sensitive/auth.go", "")
		require.NoError(t, err)
		assert.Equal(t, 50, matchCfg.BaselineConfig.MaxSuggestions)
		assert.Equal(t, 10, matchCfg.ConcurrencyLimit)

		// File not matching pattern
		noMatchCfg, err := service.ResolveEffectiveConfig(ctx, orgID, teamID, repoID, "pkg/util.go", "")
		require.NoError(t, err)
		assert.NotEqual(t, 50, noMatchCfg.BaselineConfig.MaxSuggestions)
	})

	t.Run("Enterprise Policy Guardrail Prevents Forbidden Models", func(t *testing.T) {
		orgGuard := EnterpriseCodeReviewConfig{
			Tier:           TierOrganization,
			OrganizationID: "org-guarded",
			PolicyGuardrail: EnterprisePolicyGuardrail{
				ForbiddenModels: []string{"unapproved-model-x"},
				MinStrictness:   "strict",
			},
			BaselineConfig: domain.CodeReviewConfig{
				ByokModelID: "unapproved-model-x",
			},
		}
		err := service.SaveConfig(ctx, orgGuard)
		require.NoError(t, err)

		eff, err := service.ResolveEffectiveConfig(ctx, "org-guarded", "", "", "", "")
		require.NoError(t, err)
		// Model should be reset away from unapproved-model-x
		assert.NotEqual(t, "unapproved-model-x", eff.BaselineConfig.ByokModelID)
		assert.Equal(t, domain.StrictnessStrict, eff.BaselineConfig.Strictness)
	})

	t.Run("In-Repo Config Reconciliation Blocks Illegal Bypass", func(t *testing.T) {
		ok, reason, err := service.ReconcileInRepoConfig(ctx, "org-guarded", "repo-1", "enabled: false\n")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Contains(t, reason, "prohibits disabling")
	})
}

func TestDeepDashboardAnalyticsService(t *testing.T) {
	analytics := NewDeepDashboardAnalyticsService(2 * time.Minute)
	ctx := context.Background()
	wsID := uuid.New()

	// Ingest sample review records
	now := time.Now().UTC()
	analytics.IngestReviewRecord(ReviewRecord{
		ReviewID:        uuid.New(),
		WorkspaceID:     wsID,
		RepoNamespace:   "org/auth-service",
		Duration:        45 * time.Second,
		CriticalCount:   0,
		HighCount:       1,
		MediumCount:     2,
		AcceptedExact:   2,
		AcceptedManual:  1,
		DismissedCount:  0,
		ViolatedRuleIDs: []string{"sec-jwt-01"},
		CreatedAt:       now.Add(-10 * time.Minute),
	})

	analytics.IngestReviewRecord(ReviewRecord{
		ReviewID:        uuid.New(),
		WorkspaceID:     wsID,
		RepoNamespace:   "org/billing-service",
		Duration:        90 * time.Second,
		CriticalCount:   1,
		HighCount:       0,
		MediumCount:     0,
		AcceptedExact:   1,
		AcceptedManual:  0,
		DismissedCount:  1,
		ViolatedRuleIDs: []string{"sec-sqli-02"},
		CreatedAt:       now.Add(-5 * time.Minute),
	})

	t.Run("CalculateTurnaroundMetrics", func(t *testing.T) {
		metrics := analytics.CalculateTurnaroundMetrics(ctx, wsID, 1*time.Hour)
		assert.Equal(t, 2, metrics.TotalReviewsAnalyzed)
		assert.Equal(t, 45*time.Second, metrics.P50Duration)
		assert.Equal(t, 1.0, metrics.SLAComplianceRate) // Both under 2 min threshold
	})

	t.Run("ComputeSecurityPosture", func(t *testing.T) {
		posture := analytics.ComputeSecurityPosture(ctx, wsID)
		assert.Equal(t, 1, posture.OpenCriticalCount)
		assert.Equal(t, 1, posture.OpenHighCount)
		assert.Equal(t, 4, posture.ResolvedExactCount+posture.ResolvedManualCount)
		assert.Greater(t, posture.PostureScore, 50)
		assert.NotEmpty(t, posture.TopViolatedRuleIDs)
	})

	t.Run("GenerateExecutiveSummaryMarkdown", func(t *testing.T) {
		md := analytics.GenerateExecutiveSummaryMarkdown(ctx, wsID, 1*time.Hour)
		assert.Contains(t, md, "# 🛡️ ScanDrix Code Review Executive Intelligence")
		assert.Contains(t, md, "Security Posture Score")
		assert.Contains(t, md, "SLA Target Compliance")
		assert.Contains(t, md, "`org/billing-service`")
	})
}

func TestDeepFeedbackRefinementService(t *testing.T) {
	service := NewDeepFeedbackRefinementService()
	ctx := context.Background()

	ruleID := "rule-sql-detect"

	t.Run("Healthy Rule Evaluation", func(t *testing.T) {
		for i := 0; i < 8; i++ {
			_ = service.IngestFeedback(ctx, UserSuggestionFeedback{
				RuleID: ruleID,
				Vote:   VoteHelpful,
			})
		}
		for i := 0; i < 2; i++ {
			_ = service.IngestFeedback(ctx, UserSuggestionFeedback{
				RuleID:  ruleID,
				Vote:    VoteFalsePositive,
				Snippet: "db.Query(safeConstantQuery)",
			})
		}

		metrics := service.EvaluateRuleHealth(ctx, ruleID)
		assert.Equal(t, 10, metrics.TotalFired)
		assert.Equal(t, 8, metrics.HelpfulCount)
		assert.Equal(t, 0.20, metrics.NoiseRatio)
		assert.Equal(t, "HEALTHY", metrics.RecommendedAction)
	})

	t.Run("Noisy Rule Recommends Tuning and Synthesizes Negative Regex", func(t *testing.T) {
		noisyRule := "rule-logging-audit"
		for i := 0; i < 2; i++ {
			_ = service.IngestFeedback(ctx, UserSuggestionFeedback{
				RuleID: noisyRule,
				Vote:   VoteHelpful,
			})
		}
		for i := 0; i < 8; i++ {
			_ = service.IngestFeedback(ctx, UserSuggestionFeedback{
				RuleID:  noisyRule,
				Vote:    VoteFalsePositive,
				Snippet: "logger.Infof(\"test_mock_token_123\")",
			})
		}

		metrics := service.EvaluateRuleHealth(ctx, noisyRule)
		assert.Equal(t, 10, metrics.TotalFired)
		assert.Equal(t, 0.80, metrics.NoiseRatio)
		assert.Equal(t, "AUTO_SUPPRESS", metrics.RecommendedAction)
		assert.NotEmpty(t, metrics.SynthesizedExclusion)
		assert.Contains(t, metrics.SynthesizedExclusion, "logger")
	})
}
