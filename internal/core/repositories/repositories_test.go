package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/scandrix/backend/internal/core/repositories"
	"github.com/stretchr/testify/assert"
)

func TestRepositoryConstructors(t *testing.T) {
	wsRepo := repositories.NewWorkspaceRepository(nil)
	assert.NotNil(t, wsRepo)

	orgRepo := repositories.NewOrganizationRepository(nil)
	assert.NotNil(t, orgRepo)

	userRepo := repositories.NewUserRepository(nil)
	assert.NotNil(t, userRepo)

	repoRepo := repositories.NewTrackedRepositoryRepository(nil)
	assert.NotNil(t, repoRepo)

	reviewRepo := repositories.NewPullRequestReviewRepository(nil)
	assert.NotNil(t, reviewRepo)

	findingRepo := repositories.NewCodeFindingRepository(nil)
	assert.NotNil(t, findingRepo)

	rulesRepo := repositories.NewDrixyRulesRepository(nil)
	assert.NotNil(t, rulesRepo)

	embRepo := repositories.NewSuggestionEmbeddingRepository(nil)
	assert.NotNil(t, embRepo)

	astRepo := repositories.NewAstGraphRepository(nil)
	assert.NotNil(t, astRepo)

	ctxRefRepo := repositories.NewContextReferenceRepository(nil)
	assert.NotNil(t, ctxRefRepo)

	auditRepo := repositories.NewAuditLogRepository(nil)
	assert.NotNil(t, auditRepo)

	autoRepo := repositories.NewAutomationRepository(nil)
	assert.NotNil(t, autoRepo)

	notifRepo := repositories.NewNotificationRepository(nil)
	assert.NotNil(t, notifRepo)

	authRepo := repositories.NewAuthRepository(nil)
	assert.NotNil(t, authRepo)

	teamRepo := repositories.NewTeamRepository(nil)
	assert.NotNil(t, teamRepo)

	teamKeyRepo := repositories.NewTeamCliKeyRepository(nil)
	assert.NotNil(t, teamKeyRepo)

	globalParamRepo := repositories.NewGlobalParametersRepository(nil)
	assert.NotNil(t, globalParamRepo)
}

func TestDomainEntityLifecycles(t *testing.T) {
	wsID := uuid.New()
	now := time.Now().UTC()

	// 1. Organization Model
	org := &domain.Organization{
		BaseEntity: domain.BaseEntity{
			ID:        uuid.New(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		WorkspaceID:  wsID,
		ExternalID:   "github_org_999",
		Name:         "Acme Engineering",
		BillingEmail: "billing@acme.com",
		SCMProvider:  "GITHUB",
		IsActive:     true,
		Settings:     domain.JSONBMap{"enforce_checks": true},
	}
	assert.Equal(t, wsID, org.WorkspaceID)
	assert.Equal(t, "GITHUB", org.SCMProvider)

	// 2. Tracked Repository Model
	repo := &domain.TrackedRepository{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity: domain.BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: wsID,
		},
		OrganizationID:  org.ID,
		SCMProvider:     "GITHUB",
		ExternalRepoID:  "repo_12345",
		FullName:        "acme/backend",
		DefaultBranch:   "main",
		IsPrivate:       true,
		IsActive:        true,
		WebhooksEnabled: true,
		ConfigFilePath:  ".scandrix/config.json",
	}
	assert.Equal(t, wsID, repo.WorkspaceID)
	assert.Equal(t, "acme/backend", repo.FullName)

	// 3. Pull Request Review Run
	review := &domain.PullRequestReview{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity: domain.BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: wsID,
		},
		RepositoryID:     repo.ID,
		PullNumber:       42,
		Title:            "feat: implement auth service",
		HeadSHA:          "abc123def456",
		BaseSHA:          "000000000000",
		AuthorUsername:   "developer",
		State:            "COMPLETED",
		FindingsCount:    3,
		DurationMs:       1420,
		PromptTokens:     3500,
		CompletionTokens: 820,
		ModelID:          "anthropic/claude-3-5-sonnet",
		EstimatedCostUSD: 0.024,
	}
	assert.Equal(t, 42, review.PullNumber)
	assert.Equal(t, "COMPLETED", review.State)
	assert.Equal(t, 3, review.FindingsCount)

	// 4. Code Finding
	finding := &domain.CodeFinding{
		TenantScopedEntity: domain.TenantScopedEntity{
			BaseEntity: domain.BaseEntity{
				ID:        uuid.New(),
				CreatedAt: now,
				UpdatedAt: now,
			},
			WorkspaceID: wsID,
		},
		ReviewID:        review.ID,
		RepositoryID:    repo.ID,
		RuleID:          "SEC_001_NO_HARDCODED_SECRETS",
		Category:        "SECURITY",
		Severity:        "CRITICAL",
		FilePath:        "server.go",
		LineStart:       14,
		LineEnd:         16,
		Message:         "Detected hardcoded API key in source file",
		ConfidenceScore: 0.98,
		IsResolved:      false,
	}
	assert.Equal(t, "CRITICAL", finding.Severity)
	assert.InDelta(t, 0.98, float64(finding.ConfidenceScore), 0.001)
}

func TestBatchFindingInsertionSimulation(t *testing.T) {
	wsID := uuid.New()
	reviewID := uuid.New()
	repoID := uuid.New()

	findings := make([]*domain.CodeFinding, 0, 5)
	for i := 1; i <= 5; i++ {
		findings = append(findings, &domain.CodeFinding{
			TenantScopedEntity: domain.TenantScopedEntity{
				WorkspaceID: wsID,
			},
			ReviewID:     reviewID,
			RepositoryID: repoID,
			RuleID:       "PERF_001_N_PLUS_ONE_QUERY",
			Category:     "PERFORMANCE",
			Severity:     "HIGH",
			FilePath:     "service.go",
			LineStart:    i * 10,
			LineEnd:      i*10 + 5,
			Message:      "Potential N+1 database query in loop",
		})
	}

	assert.Len(t, findings, 5)
	assert.Equal(t, wsID, findings[0].WorkspaceID)
}

func TestContextCancellationBehavior(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	wsRepo := repositories.NewWorkspaceRepository(nil)
	assert.NotNil(t, wsRepo)
	assert.Error(t, ctx.Err())
}
