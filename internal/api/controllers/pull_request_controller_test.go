package controllers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/platformdata/application/usecases"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPullRequestRepo struct {
	executions models.PaginatedEnrichedPullRequests
	digest     models.PullRequestsDailyDigest
	facets     models.PullRequestsFacets
	authors    []models.PullRequestAuthorSuggestion
	awaiting   []models.AwaitingPullRequest
	files      []models.PullRequestChangedFile
	findings   []models.CodeFinding
}

func (m *mockPullRequestRepo) ListPullRequestExecutions(ctx context.Context, wsID uuid.UUID, filter models.PullRequestExecutionFilter) (*models.PaginatedEnrichedPullRequests, error) {
	return &m.executions, nil
}

func (m *mockPullRequestRepo) GetPullRequestDailyDigest(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) (*models.PullRequestsDailyDigest, error) {
	return &m.digest, nil
}

func (m *mockPullRequestRepo) GetPullRequestFacets(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, scope string, userEmail string) (*models.PullRequestsFacets, error) {
	return &m.facets, nil
}

func (m *mockPullRequestRepo) GetPullRequestAuthors(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, search string, limit int) ([]models.PullRequestAuthorSuggestion, error) {
	return m.authors, nil
}

func (m *mockPullRequestRepo) GetAwaitingPullRequests(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) ([]models.AwaitingPullRequest, error) {
	return m.awaiting, nil
}

func (m *mockPullRequestRepo) GetPullRequestChangedFiles(ctx context.Context, wsID, repoID uuid.UUID, prNumber int) ([]models.PullRequestChangedFile, error) {
	return m.files, nil
}

func (m *mockPullRequestRepo) GetReviewFindings(ctx context.Context, reviewID uuid.UUID, optionalWsID ...uuid.UUID) ([]models.CodeFinding, error) {
	return m.findings, nil
}

func TestPullRequestController_Endpoints(t *testing.T) {
	wsID := uuid.New()
	repoID := uuid.New()

	mockRepo := &mockPullRequestRepo{
		executions: models.PaginatedEnrichedPullRequests{
			Data: []models.EnrichedPullRequestExecution{
				{
					UUID:              uuid.New(),
					RepositoryID:      repoID.String(),
					RepositoryName:    "acme/backend",
					PullRequestNumber: 42,
					PullRequestTitle:  "feat: enterprise auth",
					Author:            "dev@acme.com",
					Status:            "success",
					CreatedAt:         time.Now().UTC(),
					CompletedAt:       func() *time.Time { t := time.Now().UTC(); return &t }(),
					SuggestionsCount:  2,
					CriticalCount:     1,
				},
			},
			Total:      1,
			Page:       1,
			Limit:      20,
			TotalPages: 1,
		},
		digest: models.PullRequestsDailyDigest{
			ReviewedCount:       5,
			NeedsAttentionCount: 1,
			ErroredCount:        0,
			AwaitingCount:       2,
		},
		facets: models.PullRequestsFacets{
			All:            10,
			NeedsAttention: 2,
			Errored:        1,
			Awaiting:       3,
			Mine:           4,
		},
		authors: []models.PullRequestAuthorSuggestion{
			{Author: "dev@acme.com", Count: 15},
		},
		awaiting: []models.AwaitingPullRequest{
			{RepositoryID: repoID.String(), PullRequestNumber: 43, PullRequestTitle: "fix: memory leak"},
		},
		files: []models.PullRequestChangedFile{
			{FilePath: "auth/jwt.go", Status: "modified", Additions: 25, Deletions: 5},
		},
		findings: []models.CodeFinding{
			{
				ID:          uuid.New(),
				FilePath:    "auth/jwt.go",
				StartLine:   10,
				EndLine:     15,
				Severity:    models.SeverityCritical,
				Category:    "security",
				Title:       "Hardcoded JWT Secret",
				Description: "Secret should be read from environment",
			},
		},
	}

	backfillUC := usecases.NewBackfillHistoricalPRsUseCase(repositories.NewMemoryPullRequestsRepository(), nil, nil)
	ctrl := controllers.NewPullRequestController(mockRepo, nil).WithBackfillUseCase(backfillUC)
	r := chi.NewRouter()

	// Inject workspace middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/pull-requests", ctrl.Routes())

	// 1. Test /pull-requests/executions
	req := httptest.NewRequest("GET", "/pull-requests/executions?page=1&limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "acme/backend")

	// 2. Test /pull-requests/executions/summary
	req = httptest.NewRequest("GET", "/pull-requests/executions/summary", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "reviewed_count")

	// 3. Test /pull-requests/executions/facets
	req = httptest.NewRequest("GET", "/pull-requests/executions/facets", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "needs_attention")

	// 4. Test /pull-requests/awaiting
	req = httptest.NewRequest("GET", "/pull-requests/awaiting", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "fix: memory leak")

	// 5. Test /pull-requests/authors
	req = httptest.NewRequest("GET", "/pull-requests/authors?q=dev", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "dev@acme.com")

	// 6. Test /pull-requests/files
	req = httptest.NewRequest("GET", "/pull-requests/files?repositoryId="+repoID.String()+"&prNumber=42", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "auth/jwt.go")

	// 7. Test /pull-requests/suggestions (json format)
	req = httptest.NewRequest("GET", "/pull-requests/suggestions?prNumber=42", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Hardcoded JWT Secret")

	// 8. Test /pull-requests/suggestions (markdown format for CLI)
	req = httptest.NewRequest("GET", "/pull-requests/suggestions?prNumber=42&format=markdown", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "# ScanDrix Code Review Suggestions")

	// 9. Test /pull-requests/backfill
	backfillBody := `{"repositoryIds":["` + repoID.String() + `"],"startDate":"2026-01-01","endDate":"2026-09-01"}`
	req = httptest.NewRequest("POST", "/pull-requests/backfill", strings.NewReader(backfillBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	assert.Contains(t, w.Body.String(), "PR backfill started in background")
}
