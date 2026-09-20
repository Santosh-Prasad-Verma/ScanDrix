package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/application/usecases/dashboard"
	"github.com/scandrix/backend/pkg/models"
)

type mockDashboardRepo struct {
	facets   *models.PullRequestsFacets
	digest   *models.PullRequestsDailyDigest
	authors  []models.PullRequestAuthorSuggestion
	awaiting []models.AwaitingPullRequest
	prs      *models.PaginatedEnrichedPullRequests
}

func (m *mockDashboardRepo) ListPullRequestExecutions(
	ctx context.Context,
	wsID uuid.UUID,
	filter models.PullRequestExecutionFilter,
) (*models.PaginatedEnrichedPullRequests, error) {
	if m.prs != nil {
		return m.prs, nil
	}
	return &models.PaginatedEnrichedPullRequests{
		Data:       []models.EnrichedPullRequestExecution{},
		Total:      0,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: 0,
	}, nil
}

func (m *mockDashboardRepo) GetPullRequestDailyDigest(
	ctx context.Context,
	wsID uuid.UUID,
	teamID *uuid.UUID,
) (*models.PullRequestsDailyDigest, error) {
	if m.digest != nil {
		return m.digest, nil
	}
	return &models.PullRequestsDailyDigest{}, nil
}

func (m *mockDashboardRepo) GetPullRequestFacets(
	ctx context.Context,
	wsID uuid.UUID,
	teamID *uuid.UUID,
	scope string,
	userEmail string,
) (*models.PullRequestsFacets, error) {
	if m.facets != nil {
		return m.facets, nil
	}
	return &models.PullRequestsFacets{}, nil
}

func (m *mockDashboardRepo) GetPullRequestAuthors(
	ctx context.Context,
	wsID uuid.UUID,
	teamID *uuid.UUID,
	search string,
	limit int,
) ([]models.PullRequestAuthorSuggestion, error) {
	if m.authors != nil {
		return m.authors, nil
	}
	return []models.PullRequestAuthorSuggestion{}, nil
}

func (m *mockDashboardRepo) GetAwaitingPullRequests(
	ctx context.Context,
	wsID uuid.UUID,
	teamID *uuid.UUID,
) ([]models.AwaitingPullRequest, error) {
	if m.awaiting != nil {
		return m.awaiting, nil
	}
	return []models.AwaitingPullRequest{}, nil
}

func TestGetFacetsUseCase(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	mockRepo := &mockDashboardRepo{
		facets: &models.PullRequestsFacets{
			All:            42,
			NeedsAttention: 7,
			Errored:        2,
			Awaiting:       3,
			Mine:           12,
		},
	}

	uc := dashboard.NewGetFacetsUseCase(mockRepo)
	res, err := uc.Execute(ctx, dashboard.FacetsQuery{
		WorkspaceID: wsID,
		Scope:       "team",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.All != 42 || res.NeedsAttention != 7 || res.Mine != 12 {
		t.Fatalf("unexpected facets: %+v", res)
	}

	// Missing workspace ID check
	_, err = uc.Execute(ctx, dashboard.FacetsQuery{})
	if err == nil {
		t.Fatal("expected error on empty workspace ID")
	}
}

func TestGetDailyDigestUseCase(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	mockRepo := &mockDashboardRepo{
		digest: &models.PullRequestsDailyDigest{
			ReviewedCount:       15,
			NeedsAttentionCount: 3,
			ErroredCount:        1,
			AwaitingCount:       2,
		},
	}

	uc := dashboard.NewGetDailyDigestUseCase(mockRepo)
	res, err := uc.Execute(ctx, dashboard.DailyDigestQuery{
		WorkspaceID: wsID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ReviewedCount != 15 || res.NeedsAttentionCount != 3 {
		t.Fatalf("unexpected daily digest: %+v", res)
	}
}

func TestGetAwaitingPullRequestsUseCase(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()
	now := time.Now().UTC()

	mockRepo := &mockDashboardRepo{
		awaiting: []models.AwaitingPullRequest{
			{
				RepositoryID:      uuid.New().String(),
				RepositoryName:    "backend",
				PullRequestNumber: 101,
				PullRequestTitle:  "Feature A",
				Author:            "alice",
				CreatedAt:         now.Add(-2 * time.Hour),
			},
			{
				RepositoryID:      uuid.New().String(),
				RepositoryName:    "frontend",
				PullRequestNumber: 202,
				PullRequestTitle:  "Feature B",
				Author:            "bob",
				CreatedAt:         now.Add(-10 * time.Minute), // newer
			},
		},
	}

	uc := dashboard.NewGetAwaitingPullRequestsUseCase(mockRepo)
	res, err := uc.Execute(ctx, dashboard.AwaitingQuery{
		WorkspaceID: wsID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 2 {
		t.Fatalf("expected 2 awaiting PRs, got %d", len(res))
	}

	// Should be sorted newest first (Feature B before Feature A)
	if res[0].PullRequestNumber != 202 || res[1].PullRequestNumber != 101 {
		t.Fatalf("expected newest PR first, got: %d then %d", res[0].PullRequestNumber, res[1].PullRequestNumber)
	}
}

func TestGetPullRequestAuthorsUseCase(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	mockRepo := &mockDashboardRepo{
		authors: []models.PullRequestAuthorSuggestion{
			{Author: "alice", Count: 14},
			{Author: "bob", Count: 8},
		},
	}

	uc := dashboard.NewGetPullRequestAuthorsUseCase(mockRepo)
	res, err := uc.Execute(ctx, dashboard.AuthorsQuery{
		WorkspaceID: wsID,
		Search:      "ali",
		Limit:       5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 2 || res[0].Author != "alice" {
		t.Fatalf("unexpected authors result: %+v", res)
	}
}

func TestGetEnrichedPullRequestsUseCase(t *testing.T) {
	ctx := context.Background()
	wsID := uuid.New()

	mockRepo := &mockDashboardRepo{
		prs: &models.PaginatedEnrichedPullRequests{
			Data: []models.EnrichedPullRequestExecution{
				{
					UUID:              uuid.New(),
					RepositoryName:    "core",
					PullRequestNumber: 42,
					PullRequestTitle:  "Auth fix",
					Author:            "alice",
					Status:            "success",
					SuggestionsCount:  3,
					CriticalCount:     1,
				},
			},
			Total:      1,
			Page:       1,
			Limit:      30,
			TotalPages: 1,
		},
	}

	uc := dashboard.NewGetEnrichedPullRequestsUseCase(mockRepo)
	res, err := uc.Execute(ctx, dashboard.EnrichedPullRequestsQuery{
		WorkspaceID: wsID,
		Page:        1,
		Limit:       30,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Total != 1 || len(res.Data) != 1 || res.Data[0].PullRequestNumber != 42 {
		t.Fatalf("unexpected enriched PR result: %+v", res)
	}
}
