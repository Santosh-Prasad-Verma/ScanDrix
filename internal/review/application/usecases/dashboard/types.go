// Package dashboard provides use cases for pull request review dashboard analytics, facets, and reporting.
package dashboard

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// EnrichedPullRequestsQuery encapsulates query filters for listing enriched pull request executions.
type EnrichedPullRequestsQuery struct {
	WorkspaceID       uuid.UUID  `json:"workspace_id"`
	TeamID            *uuid.UUID `json:"team_id,omitempty"`
	RepositoryID      *uuid.UUID `json:"repository_id,omitempty"`
	RepositoryName    string     `json:"repository_name,omitempty"`
	Author            string     `json:"author,omitempty"`
	AuthorPolicy      string     `json:"author_policy,omitempty"` // "all", "reviewable", "ignored"
	Status            string     `json:"status,omitempty"`        // "success", "error", "in_progress"
	HasSentSuggestions *bool     `json:"has_sent_suggestions,omitempty"`
	NeedsAttention    *bool      `json:"needs_attention,omitempty"`
	SearchTitle       string     `json:"search_title,omitempty"`
	PullRequestNumber *int       `json:"pull_request_number,omitempty"`
	CreatedAtFrom     *time.Time `json:"created_at_from,omitempty"`
	CreatedAtTo       *time.Time `json:"created_at_to,omitempty"`
	Severity          string     `json:"severity,omitempty"`
	Category          string     `json:"category,omitempty"`
	Page              int        `json:"page"`
	Limit             int        `json:"limit"`
}

// FacetsQuery defines parameters for retrieving dashboard segment counts.
type FacetsQuery struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	TeamID      *uuid.UUID `json:"team_id,omitempty"`
	Scope       string     `json:"scope,omitempty"` // "mine" or "team"
	UserEmail   string     `json:"user_email,omitempty"`
}

// DailyDigestQuery defines parameters for retrieving today's aggregated review metrics.
type DailyDigestQuery struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	TeamID      *uuid.UUID `json:"team_id,omitempty"`
}

// AuthorsQuery defines parameters for retrieving autocomplete author suggestions.
type AuthorsQuery struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	TeamID      *uuid.UUID `json:"team_id,omitempty"`
	Search      string     `json:"search,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}

// AwaitingQuery defines parameters for retrieving open pull requests awaiting review.
type AwaitingQuery struct {
	WorkspaceID uuid.UUID  `json:"workspace_id"`
	TeamID      *uuid.UUID `json:"team_id,omitempty"`
	Limit       int        `json:"limit,omitempty"`
}

// IDashboardExecutionRepository defines the storage boundary for dashboard analytics and executions.
type IDashboardExecutionRepository interface {
	ListPullRequestExecutions(ctx context.Context, wsID uuid.UUID, filter models.PullRequestExecutionFilter) (*models.PaginatedEnrichedPullRequests, error)
	GetPullRequestDailyDigest(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) (*models.PullRequestsDailyDigest, error)
	GetPullRequestFacets(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, scope string, userEmail string) (*models.PullRequestsFacets, error)
	GetPullRequestAuthors(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, search string, limit int) ([]models.PullRequestAuthorSuggestion, error)
	GetAwaitingPullRequests(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID) ([]models.AwaitingPullRequest, error)
}
