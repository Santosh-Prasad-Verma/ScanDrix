package usecases

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// DashboardFilters parameters for filtering review dashboard views.
type DashboardFilters struct {
	RepositoryID   *uuid.UUID
	AuthorUsername string
	State          string
	SearchQuery    string
	FromDate       *time.Time
	ToDate         *time.Time
}

// EnrichedPullRequest decorates a PR review record with findings breakdown and duration.
type EnrichedPullRequest struct {
	Review       models.PullRequestReview `json:"review"`
	Repository   string                   `json:"repository"`
	Criticals    int                      `json:"criticals"`
	Majors       int                      `json:"majors"`
	Minors       int                      `json:"minors"`
	DurationSecs float64                  `json:"durationSecs"`
}

// AwaitingPullRequest represents a PR waiting for review.
type AwaitingPullRequest struct {
	PullRequestID   uuid.UUID     `json:"pullRequestId"`
	Number          int           `json:"number"`
	Title           string        `json:"title"`
	Author          string        `json:"author"`
	HeadBranch      string        `json:"headBranch"`
	BaseBranch      string        `json:"baseBranch"`
	AwaitingSince   time.Time     `json:"awaitingSince"`
	EstimatedReview time.Duration `json:"estimatedReview"`
}

// PullRequestsDailyDigest provides aggregated summary statistics.
type PullRequestsDailyDigest struct {
	Date                time.Time `json:"date"`
	TotalReviewed       int       `json:"totalReviewed"`
	SuggestionsCreated  int       `json:"suggestionsCreated"`
	SuggestionsResolved int       `json:"suggestionsResolved"`
	AverageDurationSec  float64   `json:"averageDurationSec"`
}

// PullRequestsFacets provides review categorization metrics.
type PullRequestsFacets struct {
	All            int `json:"all"`
	NeedsAttention int `json:"needsAttention"`
	Errored        int `json:"errored"`
	Awaiting       int `json:"awaiting"`
	Mine           int `json:"mine"`
}

// IDashboardRepository provides querying across pull request review runs and findings.
type IDashboardRepository interface {
	ListReviews(ctx context.Context, workspaceID uuid.UUID, limit, offset int, filters DashboardFilters) ([]models.PullRequestReview, error)
	CountReviews(ctx context.Context, workspaceID uuid.UUID, filters DashboardFilters) (int, error)
	ListDistinctAuthors(ctx context.Context, workspaceID uuid.UUID) ([]string, error)
	GetDailyDigest(ctx context.Context, workspaceID uuid.UUID, dayStart time.Time) (*PullRequestsDailyDigest, error)
	GetFacets(ctx context.Context, workspaceID uuid.UUID, currentUsername string) (*PullRequestsFacets, error)
	ListAwaiting(ctx context.Context, workspaceID uuid.UUID, limit int) ([]AwaitingPullRequest, error)
	GetFindingsBreakdown(ctx context.Context, reviewID uuid.UUID) (criticals, majors, minors int, err error)
}

// DashboardUseCases coordinates review analytics and dashboard feeds.
type DashboardUseCases struct {
	repo IDashboardRepository
}

// NewDashboardUseCases creates a new dashboard use cases coordinator.
func NewDashboardUseCases(repo IDashboardRepository) *DashboardUseCases {
	return &DashboardUseCases{repo: repo}
}

// GetEnrichedPullRequests retrieves paginated pull request reviews with findings summaries.
func (u *DashboardUseCases) GetEnrichedPullRequests(ctx context.Context, workspaceID uuid.UUID, limit, offset int, filters DashboardFilters) ([]EnrichedPullRequest, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	reviews, err := u.repo.ListReviews(ctx, workspaceID, limit, offset, filters)
	if err != nil {
		return nil, 0, err
	}

	total, err := u.repo.CountReviews(ctx, workspaceID, filters)
	if err != nil {
		return nil, 0, err
	}

	enriched := make([]EnrichedPullRequest, 0, len(reviews))
	for _, r := range reviews {
		crit, maj, min, _ := u.repo.GetFindingsBreakdown(ctx, r.ID)
		duration := 0.0
		if r.CompletedAt != nil {
			duration = r.CompletedAt.Sub(r.CreatedAt).Seconds()
		}

		enriched = append(enriched, EnrichedPullRequest{
			Review:       r,
			Repository:   r.RepositoryID.String(),
			Criticals:    crit,
			Majors:       maj,
			Minors:       min,
			DurationSecs: duration,
		})
	}

	return enriched, total, nil
}

// GetAwaitingPullRequests lists open PRs awaiting first review.
func (u *DashboardUseCases) GetAwaitingPullRequests(ctx context.Context, workspaceID uuid.UUID, limit int) ([]AwaitingPullRequest, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	return u.repo.ListAwaiting(ctx, workspaceID, limit)
}

// GetPullRequestAuthors returns distinct PR authors for filter dropdowns.
func (u *DashboardUseCases) GetPullRequestAuthors(ctx context.Context, workspaceID uuid.UUID) ([]string, error) {
	return u.repo.ListDistinctAuthors(ctx, workspaceID)
}

// GetDailyDigest calculates today's review counts, issues, and attention items.
func (u *DashboardUseCases) GetDailyDigest(ctx context.Context, workspaceID uuid.UUID) (*PullRequestsDailyDigest, error) {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	digest, err := u.repo.GetDailyDigest(ctx, workspaceID, startOfDay)
	if err != nil {
		return nil, err
	}
	if digest == nil {
		return &PullRequestsDailyDigest{
			Date: startOfDay,
		}, nil
	}
	return digest, nil
}

// GetPullRequestsFacets returns filter badge totals (all, needsAttention, errored, awaiting, mine).
func (u *DashboardUseCases) GetPullRequestsFacets(ctx context.Context, workspaceID uuid.UUID, currentUsername string) (*PullRequestsFacets, error) {
	return u.repo.GetFacets(ctx, workspaceID, currentUsername)
}
