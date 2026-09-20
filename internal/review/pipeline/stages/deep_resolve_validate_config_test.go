package stages

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

type mockSlotResolver struct {
	slot *domain.ResolvedModelSlotInfo
}

func (m *mockSlotResolver) ResolveTaskSlot(ctx context.Context, orgID, taskName, modelOverride string) (*domain.ResolvedModelSlotInfo, error) {
	return m.slot, nil
}

type mockConfigResolver struct {
	cfg  domain.CodeReviewConfig
	msgs *domain.PullRequestMessages
}

func (m *mockConfigResolver) GetCodeReviewParameter(ctx context.Context, orgID, teamID, repoID string) (domain.CodeReviewConfig, error) {
	return m.cfg, nil
}

func (m *mockConfigResolver) FindByRepoOrDirectory(ctx context.Context, orgID, repoID, dirID string) (*domain.PullRequestMessages, error) {
	return m.msgs, nil
}

func TestDeepResolveConfigStage_InRepoOverrideAndSlotResolution(t *testing.T) {
	ctx := context.Background()

	baseCfg := domain.DefaultCodeReviewConfig()
	baseCfg.MaxSuggestions = 10

	cr := &mockConfigResolver{cfg: baseCfg}
	sr := &mockSlotResolver{
		slot: &domain.ResolvedModelSlotInfo{
			ModelID:   "claude-3-7-sonnet",
			ModelName: "Claude 3.7 Sonnet",
			Provider:  "anthropic",
		},
	}

	stage := NewDeepResolveConfigStage(cr, sr)

	// Simulated committed `.scandrix/config.json` in diff patches
	inRepoJSON := `{"enabled": true, "maxSuggestions": 40, "strictness": "STRICT", "byokModelId": "claude-3-7-sonnet"}`
	patch := &diff.FilePatch{
		NewPath: ".scandrix/config.json",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: inRepoJSON},
				},
			},
		},
	}

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		ParsedPatches: []*diff.FilePatch{patch},
	}

	if err := stage.Execute(ctx, pCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In-repo config overrode base config maxSuggestions from 10 to 40
	if pCtx.ResolvedConfig.MaxSuggestions != 40 {
		t.Errorf("expected maxSuggestions 40 from in-repo config, got %d", pCtx.ResolvedConfig.MaxSuggestions)
	}
	if pCtx.ResolvedConfig.Strictness != domain.StrictnessStrict {
		t.Errorf("expected strictness STRICT, got %s", pCtx.ResolvedConfig.Strictness)
	}
	if pCtx.ResolvedConfig.ResolvedModelSlot == nil || pCtx.ResolvedConfig.ResolvedModelSlot.ModelID != "claude-3-7-sonnet" {
		t.Errorf("expected resolved model slot claude-3-7-sonnet")
	}
}

func TestDeepValidateConfigStage_DisabledAndDraftGating(t *testing.T) {
	ctx := context.Background()
	stage := NewDeepValidateConfigStage(nil, nil)

	// 1. Review disabled
	pCtx := &pipeline.PipelineContext{
		ResolvedConfig: domain.CodeReviewConfig{Enabled: false},
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "CONFIG_DISABLED" {
		t.Fatalf("expected disabled config to skip review")
	}

	// 2. Draft PR skipped
	pCtx = &pipeline.PipelineContext{
		IsDraft: true,
		ResolvedConfig: domain.CodeReviewConfig{
			Enabled:               true,
			AutomatedReviewActive: true,
			RunOnDraft:            false,
		},
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "DRAFT_PR_SKIPPED" {
		t.Fatalf("expected draft PR to be skipped")
	}

	// 3. Draft PR with command bypass
	pCtx.Origin = "command"
	pCtx.SkipReview = false
	pCtx.StatusInfo = pipeline.PipelineStatusInfo{}
	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected command origin to bypass draft skip")
	}
}

func TestDeepValidateConfigStage_BranchMismatchAndCadenceAutoPause(t *testing.T) {
	ctx := context.Background()
	fb := &mockFeedbackReaction{}
	now := time.Now()
	cadenceStore := &mockCadenceStore{
		lastState: domain.CadenceStateAutomatic,
		recentRuns: []time.Time{
			now.Add(-2 * time.Minute),
			now.Add(-4 * time.Minute),
			now.Add(-6 * time.Minute),
		},
	}
	cadenceEngine := NewReviewCadenceEngine(cadenceStore)
	stage := NewDeepValidateConfigStage(cadenceEngine, fb)

	// 1. Branch mismatch: configured only for main, but targeting staging
	pCtx := &pipeline.PipelineContext{
		Branch:     "feature/login",
		BaseBranch: "staging",
		Provider:   models.ProviderGitHub,
		ResolvedConfig: domain.CodeReviewConfig{
			Enabled:               true,
			AutomatedReviewActive: true,
			BaseBranches:          []string{"main", "!staging"},
		},
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "BRANCH_MISMATCH" {
		t.Fatalf("expected branch mismatch to skip review")
	}

	// 2. Review cadence auto-pause on burst pushes
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		WorkspaceID:  uuid.New(),
		PullNumber:   42,
		Branch:       "feature/login",
		BaseBranch:   "main",
		Provider:     models.ProviderGitHub,
		LastExecution: &pipeline.PreviousExecutionInfo{
			LastAnalyzedCommit: "c_prev",
		},
		ResolvedConfig: domain.CodeReviewConfig{
			Enabled:               true,
			AutomatedReviewActive: true,
			BaseBranches:          []string{"main"},
			ReviewCadence: domain.ReviewCadenceConfig{
				Type:              domain.CadenceAutoPause,
				PushesToTrigger:   3,
				TimeWindowMinutes: 15,
			},
		},
	}

	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "CADENCE_SUPPRESSED" {
		t.Fatalf("expected burst pushes to trigger cadence auto-pause")
	}

	if len(fb.noticesPosted) == 0 || !strings.Contains(fb.noticesPosted[0], "Auto-paused") {
		t.Errorf("expected auto-pause comment to be posted to SCM")
	}
}

func TestDeepResolveConfigStage_YAMLAndMechanicalDetectorFindings(t *testing.T) {
	ctx := context.Background()
	baseCfg := domain.DefaultCodeReviewConfig()
	cr := &mockConfigResolver{cfg: baseCfg}
	stage := NewDeepResolveConfigStage(cr, nil)

	yamlConfig := `
version: "1.0"
review_mode: "deep"
sensitivity: "strict"
max_comments_per_review: 50
committable_suggestions: true
ignored_file_patterns:
  - "dist/**"
included_branch_patterns:
  - "main"
`
	configPatch := &diff.FilePatch{
		NewPath: ".scandrix.yml",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, Content: yamlConfig},
				},
			},
		},
	}

	codePatch := &diff.FilePatch{
		NewPath: "pkg/auth/credentials.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, Content: "package auth", OldLineNo: 1, NewLineNo: 1},
					{Type: diff.LineAddition, Content: `const awsToken = "AKIAIOSFODNN7ABCD123"`, NewLineNo: 2},
				},
			},
		},
	}

	pCtx := &pipeline.PipelineContext{
		WorkspaceID:   uuid.New(),
		RepositoryID:  uuid.New(),
		ParsedPatches: []*diff.FilePatch{configPatch, codePatch},
	}

	err := stage.Execute(ctx, pCtx)
	if err != nil {
		t.Fatalf("unexpected error executing stage: %v", err)
	}

	// Verify in-repo config override
	if pCtx.ResolvedConfig.ReviewMode != "deep" {
		t.Errorf("expected ReviewMode 'deep', got %s", pCtx.ResolvedConfig.ReviewMode)
	}
	if pCtx.ResolvedConfig.Sensitivity != "STRICT" {
		t.Errorf("expected Sensitivity 'STRICT', got %s", pCtx.ResolvedConfig.Sensitivity)
	}
	if pCtx.ResolvedConfig.Strictness != domain.StrictnessStrict {
		t.Errorf("expected Strictness STRICT, got %s", pCtx.ResolvedConfig.Strictness)
	}
	if pCtx.ResolvedConfig.MaxSuggestions != 50 {
		t.Errorf("expected MaxSuggestions 50, got %d", pCtx.ResolvedConfig.MaxSuggestions)
	}
	if !pCtx.ResolvedConfig.EnableCommittableSuggestions {
		t.Errorf("expected EnableCommittableSuggestions true")
	}

	// Verify mechanical detection populated StaticFindings
	if len(pCtx.StaticFindings) == 0 {
		t.Fatalf("expected mechanical static finding for hardcoded secret, got none")
	}

	foundSecretRule := false
	for _, sf := range pCtx.StaticFindings {
		titleLower := strings.ToLower(sf.Title)
		if strings.Contains(titleLower, "credential") || strings.Contains(titleLower, "api key") || strings.Contains(titleLower, "secret") {
			foundSecretRule = true
			if sf.Severity != models.SeverityCritical {
				t.Errorf("expected Critical severity for hardcoded AWS secret, got %s", sf.Severity)
			}
			if sf.StartLine != 2 {
				t.Errorf("expected finding on line 2, got %d", sf.StartLine)
			}
			break
		}
	}
	if !foundSecretRule {
		t.Errorf("expected finding matching secret detection rule, got titles: %v", func() []string {
			var titles []string
			for _, f := range pCtx.StaticFindings {
				titles = append(titles, f.Title)
			}
			return titles
		}())
	}
}

