package stages

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

type mockCadenceStore struct {
	recentRuns []time.Time
	lastState  domain.ReviewCadenceState
}

func (m *mockCadenceStore) FindRecentSuccessfulRuns(ctx context.Context, repoID string, prNumber int, since time.Time) ([]time.Time, error) {
	return m.recentRuns, nil
}

func (m *mockCadenceStore) GetLastReviewCadenceState(ctx context.Context, repoID string, prNumber int) (domain.ReviewCadenceState, error) {
	return m.lastState, nil
}

func TestProcessAndValidateBranchExpression(t *testing.T) {
	// Valid expression
	validExpr := "main, !release/*, =feature/*, !==bugfix/*, contains:hotfix"
	isValid, errs := ValidateBranchExpression(validExpr)
	if !isValid || len(errs) > 0 {
		t.Fatalf("expected valid expression, got errors: %v", errs)
	}

	cfg := ProcessBranchExpression(validExpr)
	if cfg.ReviewRules == nil {
		t.Fatal("expected non-nil review rules")
	}

	// Inclusions
	if !cfg.ReviewRules["*"]["main"] {
		t.Errorf("expected 'main' to be true")
	}
	if !cfg.ReviewRules["*"]["feature/*"] {
		t.Errorf("expected 'feature/*' to be true")
	}

	// Exclusions
	if cfg.ReviewRules["*"]["!release/*"] {
		t.Errorf("expected '!release/*' to be false")
	}
	if cfg.ReviewRules["*"]["!bugfix/*"] {
		t.Errorf("expected '!bugfix/*' to be false")
	}

	// Contains
	if cfg.ReviewRules["contains:hotfix"] == nil || !cfg.ReviewRules["contains:hotfix"]["*"] {
		t.Errorf("expected 'contains:hotfix' rule present")
	}

	// Invalid expressions
	invalidExprs := []struct {
		expr string
		desc string
	}{
		{"rule1, rule1", "duplicate rule"},
		{"rule**pattern", "double wildcard"},
		{"!", "empty exclusion"},
		{"=", "empty equality"},
		{"contains:", "empty contains"},
		{"rule with spaces inside", "invalid characters"},
	}

	for _, tc := range invalidExprs {
		valid, errList := ValidateBranchExpression(tc.expr)
		if valid || len(errList) == 0 {
			t.Errorf("expected %s to fail validation: %s", tc.desc, tc.expr)
		}
	}
}

func TestConvertConfigToBranchExpression(t *testing.T) {
	cfg := BranchReviewConfig{
		ReviewRules: BranchReviewRuleMap{
			"*": {
				"main":        true,
				"!release/*": false,
			},
			"contains:urgent": {
				"*": true,
			},
		},
	}

	expr := ConvertConfigToBranchExpression(cfg)
	if !ProcessBranchExpression(expr).ReviewRules["*"]["main"] {
		t.Errorf("roundtrip conversion failed for 'main'")
	}
}

func TestShouldReviewBranches_SpecificityAndHierarchy(t *testing.T) {
	expr := "*, !main, feature/*, =release/v1.0"
	cfg := ProcessBranchExpression(expr)

	// Explicit exclusion '!main' has maximum target specificity (100)
	if ShouldReviewBranches("feature/new-ui", "main", cfg) {
		t.Errorf("expected review on target 'main' to be denied due to '!main' exclusion")
	}

	// Wildcard feature/* matches
	if !ShouldReviewBranches("feature/new-ui", "develop", cfg) {
		t.Errorf("expected review on 'develop' to be allowed by wildcard rule")
	}

	// Equality rule
	if !ShouldReviewBranches("feature/fix", "release/v1.0", cfg) {
		t.Errorf("expected review on 'release/v1.0' to be allowed")
	}

	// Contains rule
	containsCfg := ProcessBranchExpression("contains:hotfix")
	if !ShouldReviewBranches("patch/hotfix-urgent", "main", containsCfg) {
		t.Errorf("expected contains:hotfix rule to match")
	}
	if ShouldReviewBranches("patch/regular-work", "main", containsCfg) {
		t.Errorf("expected non-matching branch to be denied")
	}
}

func TestMergeBaseBranches(t *testing.T) {
	configured := []string{"develop", "!release/*", "feature/*"}
	merged := MergeBaseBranches(configured, "main")

	hasMain := false
	hasDevelop := false
	hasReleaseExclusion := false

	for _, b := range merged {
		if b == "main" {
			hasMain = true
		}
		if b == "develop" {
			hasDevelop = true
		}
		if b == "!release/*" {
			hasReleaseExclusion = true
		}
	}

	if !hasMain {
		t.Errorf("expected default branch 'main' to be merged")
	}
	if !hasDevelop {
		t.Errorf("expected configured branch 'develop' to be kept")
	}
	if !hasReleaseExclusion {
		t.Errorf("expected exclusion '!release/*' to be preserved")
	}

	// If default branch is explicitly excluded, it must NOT be injected
	configuredExcl := []string{"!main", "develop"}
	mergedExcl := MergeBaseBranches(configuredExcl, "main")
	for _, b := range mergedExcl {
		if b == "main" {
			t.Errorf("expected 'main' not to be merged when explicitly excluded with '!main'")
		}
	}
}

func TestNormalizeBranchesForPlatform_AzureDevOps(t *testing.T) {
	branches := []string{"main", "!develop", "=feature/login", "contains:hotfix"}
	normalized := NormalizeBranchesForPlatform(branches, models.ProviderAzure)

	expected := []string{
		"refs/heads/main",
		"!refs/heads/develop",
		"=refs/heads/feature/login",
		"contains:hotfix",
	}

	for i, exp := range expected {
		if normalized[i] != exp {
			t.Errorf("expected %s at %d, got %s", exp, i, normalized[i])
		}
	}

	// Non-Azure providers remain untouched
	ghNorm := NormalizeBranchesForPlatform(branches, models.ProviderGitHub)
	if ghNorm[0] != "main" {
		t.Errorf("expected GitHub branches unchanged, got %s", ghNorm[0])
	}
}

func TestReviewCadenceEngine_AllModes(t *testing.T) {
	ctx := context.Background()
	store := &mockCadenceStore{
		lastState: domain.CadenceStateAutomatic,
	}
	engine := NewReviewCadenceEngine(store)

	pCtx := &pipeline.PipelineContext{
		Origin: "webhook",
		ResolvedConfig: domain.CodeReviewConfig{
			ReviewCadence: domain.ReviewCadenceConfig{
				Type: domain.CadenceAutomatic,
			},
		},
	}

	// 1. Automatic mode
	decision, err := engine.EvaluateCadence(ctx, pCtx)
	if err != nil || !decision.ShouldProcess {
		t.Fatalf("expected automatic mode to process")
	}

	// 2. Command override (@scandrix review)
	pCtx.Origin = "command"
	pCtx.ResolvedConfig.ReviewCadence.Type = domain.CadenceManual
	pCtx.LastExecution = &pipeline.PreviousExecutionInfo{LastAnalyzedCommit: "abc1234"}
	decision, err = engine.EvaluateCadence(ctx, pCtx)
	if err != nil || !decision.ShouldProcess || decision.CurrentCadenceStatus != domain.CadenceStateCommand {
		t.Fatalf("expected command origin to bypass manual mode cadence")
	}

	// 3. Manual mode on subsequent push
	pCtx.Origin = "webhook"
	decision, err = engine.EvaluateCadence(ctx, pCtx)
	if err != nil || decision.ShouldProcess || decision.CurrentCadenceStatus != domain.CadenceStatePaused {
		t.Fatalf("expected manual mode to pause subsequent push without command")
	}

	// 4. Auto-pause mode: burst pushes detection
	now := time.Now()
	store.recentRuns = []time.Time{
		now.Add(-2 * time.Minute),
		now.Add(-5 * time.Minute),
		now.Add(-10 * time.Minute),
	}
	pCtx.ResolvedConfig.ReviewCadence.Type = domain.CadenceAutoPause
	pCtx.ResolvedConfig.ReviewCadence.PushesToTrigger = 3
	pCtx.ResolvedConfig.ReviewCadence.TimeWindowMinutes = 15

	decision, err = engine.EvaluateCadence(ctx, pCtx)
	if err != nil || decision.ShouldProcess {
		t.Fatalf("expected auto-pause to trigger on burst pushes")
	}
	if decision.PauseCommentBody == "" {
		t.Errorf("expected non-empty pause comment body")
	}
}
