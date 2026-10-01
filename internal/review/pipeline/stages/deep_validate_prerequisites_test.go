package stages

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

type mockPermissionValidator struct {
	valResult   PermissionValidationResult
	autoAssign  bool
	healedTrial bool
	hasSeat     bool
}

func (m *mockPermissionValidator) ValidateExecutionPermissions(ctx context.Context, orgID, userGitID string) (PermissionValidationResult, error) {
	return m.valResult, nil
}

func (m *mockPermissionValidator) AutoAssignLicense(ctx context.Context, orgID, userGitID string, prNumber int) (bool, string, error) {
	return m.autoAssign, "AUTO_ASSIGNED", nil
}

func (m *mockPermissionValidator) TryHealMissingTrial(ctx context.Context, orgID string) (bool, error) {
	if m.healedTrial {
		m.valResult.Allowed = true
		return true, nil
	}
	return false, nil
}

func (m *mockPermissionValidator) UserHoldsLicenseSeat(ctx context.Context, orgID, userGitID string) (bool, error) {
	return m.hasSeat, nil
}

type mockExclusionChecker struct {
	centralized bool
	globalRules bool
}

func (m *mockExclusionChecker) IsCentralizedConfigRepo(ctx context.Context, orgID, repoID string) (bool, error) {
	return m.centralized, nil
}

func (m *mockExclusionChecker) IsGlobalRulesSourceRepo(ctx context.Context, orgID, repoID string) (bool, error) {
	return m.globalRules, nil
}

type mockFeedbackReaction struct {
	reactionsPosted []string
	noticesPosted   []string
}

func (m *mockFeedbackReaction) AddReaction(ctx context.Context, repo models.TrackedRepository, prNumber int, commentID string, reaction string) error {
	m.reactionsPosted = append(m.reactionsPosted, reaction)
	return nil
}

func (m *mockFeedbackReaction) CreateNoticeComment(ctx context.Context, repo models.TrackedRepository, prNumber int, commentBody string) error {
	m.noticesPosted = append(m.noticesPosted, commentBody)
	return nil
}

func TestDeepValidatePrerequisites_StructuralAndLifecycle(t *testing.T) {
	ctx := context.Background()
	stage := NewDeepValidatePrerequisitesStage(nil, nil, nil, nil)

	// 1. Missing identifiers
	pCtx := &pipeline.PipelineContext{}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview {
		t.Fatalf("expected missing identifiers to skip review")
	}

	// 2. Closed PR (webhook)
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   10,
		PRState:      "closed",
		Origin:       "webhook",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview {
		t.Fatalf("expected closed PR to be skipped")
	}

	// 3. Closed PR with manual command override
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   10,
		PRState:      "closed",
		Origin:       "command",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected command origin to bypass closed PR skip")
	}

	// 4. Locked PR
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   10,
		IsLocked:     true,
		Origin:       "webhook",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview {
		t.Fatalf("expected locked PR to be skipped")
	}
}

func TestDeepValidatePrerequisites_BotSeatOverride(t *testing.T) {
	ctx := context.Background()
	pv := &mockPermissionValidator{hasSeat: false}
	stage := NewDeepValidatePrerequisitesStage(pv, nil, nil, nil)

	// Bot without seat is skipped
	pCtx := &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   12,
		Author:       "dependabot[bot]",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview {
		t.Fatalf("expected bot without seat to be skipped")
	}

	// Bot WITH seat is allowed!
	pv.hasSeat = true
	pv.valResult = PermissionValidationResult{Allowed: true}
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   12,
		Author:       "dependabot[bot]",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected bot holding a license seat to be reviewed")
	}
}

func TestDeepValidatePrerequisites_RepositoryExclusions(t *testing.T) {
	ctx := context.Background()
	ec := &mockExclusionChecker{centralized: true}
	stage := NewDeepValidatePrerequisitesStage(nil, ec, nil, nil)

	pCtx := &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   5,
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "CENTRALIZED_CONFIG_REPO" {
		t.Fatalf("expected centralized config repo to be excluded")
	}

	ec.centralized = false
	ec.globalRules = true
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   5,
	}
	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview || pCtx.StatusInfo.ReasonCode != "GLOBAL_RULES_SOURCE_REPO" {
		t.Fatalf("expected global rules source repo to be excluded")
	}
}

func TestDeepValidatePrerequisites_TrialHealingAndAutoAssign(t *testing.T) {
	ctx := context.Background()

	// 1. Missing trial healed
	pv := &mockPermissionValidator{
		valResult:   PermissionValidationResult{Allowed: false, ErrorType: ValidationErrorInvalidLicense},
		healedTrial: true,
	}
	stage := NewDeepValidatePrerequisitesStage(pv, nil, nil, nil)

	pCtx := &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   8,
		Author:       "developer-alice",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected healed trial to allow review execution")
	}

	// 2. User not licensed but auto-assigned
	pv = &mockPermissionValidator{
		valResult:  PermissionValidationResult{Allowed: false, ErrorType: ValidationErrorUserNotLicensed},
		autoAssign: true,
	}
	stage = NewDeepValidatePrerequisitesStage(pv, nil, nil, nil)
	pCtx = &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   9,
		Author:       "developer-bob",
	}
	if err := stage.Execute(ctx, pCtx); err != nil || pCtx.SkipReview {
		t.Fatalf("expected auto-assignment to allow review execution")
	}
}

func TestDeepValidatePrerequisites_PlanLimitAndNoticeFormatting(t *testing.T) {
	ctx := context.Background()
	fb := &mockFeedbackReaction{}
	pv := &mockPermissionValidator{
		valResult: PermissionValidationResult{
			Allowed:               false,
			ErrorType:             ValidationErrorPlanLimitReached,
			SubscriptionStatus:    "trial",
			TrialCreditsExhausted: true,
		},
	}
	stage := NewDeepValidatePrerequisitesStage(pv, nil, nil, fb)

	pCtx := &pipeline.PipelineContext{
		RepositoryID: uuid.New(),
		PullNumber:   15,
		Author:       "charlie",
		Provider:     models.ProviderGitHub,
		ResolvedConfig: domain.CodeReviewConfig{
			ShowStatusFeedback: true,
		},
	}

	if err := stage.Execute(ctx, pCtx); err != nil || !pCtx.SkipReview {
		t.Fatalf("expected exhausted trial credits to skip review")
	}

	if len(fb.reactionsPosted) == 0 || fb.reactionsPosted[0] != "-1" {
		t.Errorf("expected thumbs down reaction on GitHub, got %v", fb.reactionsPosted)
	}

	if len(fb.noticesPosted) == 0 {
		t.Fatal("expected notice comment posted")
	}

	notice := fb.noticesPosted[0]
	if !strings.Contains(notice, "https://app.scandrix.dev/byok") {
		t.Errorf("expected BYOK link in notice: %s", notice)
	}
	if !strings.Contains(notice, "<!-- drixy-codereview -->") {
		t.Errorf("expected drixy footer in notice: %s", notice)
	}
}
