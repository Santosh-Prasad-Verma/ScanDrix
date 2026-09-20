// Package database provides PostgreSQL dashboard analytics and reporting repository.
package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/review/application/usecases"
	"github.com/scandrix/backend/pkg/models"
)

// PostgresDashboardRepository implements usecases.IDashboardRepository with real PostgreSQL queries.
type PostgresDashboardRepository struct {
	repo *Repository
}

// NewPostgresDashboardRepository creates a new dashboard repository instance.
func NewPostgresDashboardRepository(repo *Repository) *PostgresDashboardRepository {
	return &PostgresDashboardRepository{repo: repo}
}

// Ensure PostgresDashboardRepository implements usecases.IDashboardRepository
var _ usecases.IDashboardRepository = (*PostgresDashboardRepository)(nil)

// ListReviews queries pull_request_reviews with filtering, pagination, and tenant isolation.
func (d *PostgresDashboardRepository) ListReviews(
	ctx context.Context,
	workspaceID uuid.UUID,
	limit, offset int,
	filters usecases.DashboardFilters,
) ([]models.PullRequestReview, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	if limit <= 0 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	var sb strings.Builder
	sb.WriteString(`
		SELECT id, workspace_id, repository_id, pull_number, title,
		       head_sha, base_sha, author_username, state, findings_count, created_at, completed_at
		FROM pull_request_reviews
		WHERE workspace_id = $1
	`)

	args := []any{workspaceID}
	paramIdx := 2

	if filters.RepositoryID != nil && *filters.RepositoryID != uuid.Nil {
		sb.WriteString(fmt.Sprintf(" AND repository_id = $%d", paramIdx))
		args = append(args, *filters.RepositoryID)
		paramIdx++
	}

	if filters.AuthorUsername != "" {
		sb.WriteString(fmt.Sprintf(" AND author_username = $%d", paramIdx))
		args = append(args, filters.AuthorUsername)
		paramIdx++
	}

	if filters.State != "" {
		sb.WriteString(fmt.Sprintf(" AND state = $%d", paramIdx))
		args = append(args, filters.State)
		paramIdx++
	}

	if filters.SearchQuery != "" {
		sb.WriteString(fmt.Sprintf(" AND (title ILIKE '%%' || $%d || '%%' OR author_username ILIKE '%%' || $%d || '%%')", paramIdx, paramIdx))
		args = append(args, filters.SearchQuery)
		paramIdx++
	}

	if filters.FromDate != nil {
		sb.WriteString(fmt.Sprintf(" AND created_at >= $%d", paramIdx))
		args = append(args, *filters.FromDate)
		paramIdx++
	}

	if filters.ToDate != nil {
		sb.WriteString(fmt.Sprintf(" AND created_at <= $%d", paramIdx))
		args = append(args, *filters.ToDate)
		paramIdx++
	}

	sb.WriteString(fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", paramIdx, paramIdx+1))
	args = append(args, limit, offset)

	var reviews []models.PullRequestReview
	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, sb.String(), args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var r models.PullRequestReview
			var stateStr string
			if err := rows.Scan(
				&r.ID, &r.WorkspaceID, &r.RepositoryID, &r.PullNumber, &r.Title,
				&r.HeadSHA, &r.BaseSHA, &r.AuthorUsername, &stateStr, &r.FindingsCount, &r.CreatedAt, &r.CompletedAt,
			); err != nil {
				return err
			}
			r.State = models.ReviewState(stateStr)
			reviews = append(reviews, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list reviews: %w", err)
	}

	return reviews, nil
}

// CountReviews counts reviews matching the specified filters.
func (d *PostgresDashboardRepository) CountReviews(
	ctx context.Context,
	workspaceID uuid.UUID,
	filters usecases.DashboardFilters,
) (int, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return 0, errors.New("database repository unavailable")
	}

	var sb strings.Builder
	sb.WriteString(`
		SELECT COUNT(*)::int
		FROM pull_request_reviews
		WHERE workspace_id = $1
	`)

	args := []any{workspaceID}
	paramIdx := 2

	if filters.RepositoryID != nil && *filters.RepositoryID != uuid.Nil {
		sb.WriteString(fmt.Sprintf(" AND repository_id = $%d", paramIdx))
		args = append(args, *filters.RepositoryID)
		paramIdx++
	}

	if filters.AuthorUsername != "" {
		sb.WriteString(fmt.Sprintf(" AND author_username = $%d", paramIdx))
		args = append(args, filters.AuthorUsername)
		paramIdx++
	}

	if filters.State != "" {
		sb.WriteString(fmt.Sprintf(" AND state = $%d", paramIdx))
		args = append(args, filters.State)
		paramIdx++
	}

	if filters.SearchQuery != "" {
		sb.WriteString(fmt.Sprintf(" AND (title ILIKE '%%' || $%d || '%%' OR author_username ILIKE '%%' || $%d || '%%')", paramIdx, paramIdx))
		args = append(args, filters.SearchQuery)
		paramIdx++
	}

	if filters.FromDate != nil {
		sb.WriteString(fmt.Sprintf(" AND created_at >= $%d", paramIdx))
		args = append(args, *filters.FromDate)
		paramIdx++
	}

	if filters.ToDate != nil {
		sb.WriteString(fmt.Sprintf(" AND created_at <= $%d", paramIdx))
		args = append(args, *filters.ToDate)
		paramIdx++
	}

	var total int
	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, sb.String(), args...).Scan(&total)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count reviews: %w", err)
	}

	return total, nil
}

// ListDistinctAuthors lists all distinct authors who submitted reviews in the workspace.
func (d *PostgresDashboardRepository) ListDistinctAuthors(ctx context.Context, workspaceID uuid.UUID) ([]string, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT DISTINCT author_username
		FROM pull_request_reviews
		WHERE workspace_id = $1 AND author_username != ''
		ORDER BY author_username ASC
	`

	var authors []string
	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var author string
			if err := rows.Scan(&author); err != nil {
				return err
			}
			authors = append(authors, author)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list distinct authors: %w", err)
	}

	return authors, nil
}

// GetDailyDigest calculates true aggregate metrics for the day using PostgreSQL CTE queries.
func (d *PostgresDashboardRepository) GetDailyDigest(
	ctx context.Context,
	workspaceID uuid.UUID,
	dayStart time.Time,
) (*usecases.PullRequestsDailyDigest, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	digest := &usecases.PullRequestsDailyDigest{
		Date: dayStart,
	}

	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		query := `
			WITH day_reviews AS (
				SELECT id, state, created_at, completed_at
				FROM pull_request_reviews
				WHERE workspace_id = $1 AND created_at >= $2
			),
			day_findings AS (
				SELECT cf.id, cf.status
				FROM code_findings cf
				JOIN day_reviews dr ON dr.id = cf.review_id
				WHERE cf.workspace_id = $1
			)
			SELECT
				COUNT(*) FILTER (WHERE state = 'COMPLETED')::int AS total_reviewed,
				(SELECT COUNT(*)::int FROM day_findings) AS suggestions_created,
				(SELECT COUNT(*)::int FROM day_findings WHERE status = 'RESOLVED') AS suggestions_resolved,
				COALESCE(
					AVG(EXTRACT(EPOCH FROM (completed_at - created_at))) 
					FILTER (WHERE state = 'COMPLETED' AND completed_at IS NOT NULL AND completed_at >= created_at),
					0.0
				)::float8 AS avg_duration_sec
			FROM day_reviews;
		`
		return tx.QueryRow(ctx, query, workspaceID, dayStart).Scan(
			&digest.TotalReviewed,
			&digest.SuggestionsCreated,
			&digest.SuggestionsResolved,
			&digest.AverageDurationSec,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query daily digest: %w", err)
	}

	return digest, nil
}

// GetFacets computes dashboard tab counts via real SQL aggregations across reviews and findings.
func (d *PostgresDashboardRepository) GetFacets(
	ctx context.Context,
	workspaceID uuid.UUID,
	currentUsername string,
) (*usecases.PullRequestsFacets, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	facets := &usecases.PullRequestsFacets{}

	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		query := `
			WITH open_with_unresolved AS (
				SELECT DISTINCT cf.review_id
				FROM code_findings cf
				JOIN pull_request_reviews pr ON pr.id = cf.review_id
				WHERE cf.workspace_id = $1
				  AND cf.status != 'RESOLVED'
				  AND COALESCE(cf.dismissal_reason, '') = ''
				  AND pr.state != 'FAILED'
			)
			SELECT
				COUNT(*)::int AS all_count,
				(SELECT COUNT(*)::int FROM open_with_unresolved) AS needs_attention,
				COUNT(*) FILTER (WHERE state = 'FAILED')::int AS errored,
				COUNT(*) FILTER (WHERE state IN ('QUEUED', 'PROCESSING', 'RECEIVED'))::int AS awaiting,
				COUNT(*) FILTER (WHERE author_username = $2 AND $2 != '')::int AS mine
			FROM pull_request_reviews
			WHERE workspace_id = $1;
		`
		return tx.QueryRow(ctx, query, workspaceID, currentUsername).Scan(
			&facets.All,
			&facets.NeedsAttention,
			&facets.Errored,
			&facets.Awaiting,
			&facets.Mine,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query facets: %w", err)
	}

	return facets, nil
}

// ListAwaiting returns pull requests queued or in processing state awaiting review.
func (d *PostgresDashboardRepository) ListAwaiting(
	ctx context.Context,
	workspaceID uuid.UUID,
	limit int,
) ([]usecases.AwaitingPullRequest, error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, pull_number, title, author_username,
		       COALESCE(base_sha, '') as base_sha, COALESCE(head_sha, '') as head_sha,
		       created_at
		FROM pull_request_reviews
		WHERE workspace_id = $1 AND state IN ('QUEUED', 'PROCESSING', 'RECEIVED')
		ORDER BY created_at ASC
		LIMIT $2
	`

	var awaiting []usecases.AwaitingPullRequest
	err := d.repo.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item usecases.AwaitingPullRequest
			if err := rows.Scan(
				&item.PullRequestID, &item.Number, &item.Title, &item.Author,
				&item.BaseBranch, &item.HeadBranch, &item.AwaitingSince,
			); err != nil {
				return err
			}
			// Estimate ~45s per review based on average pipeline latency
			item.EstimatedReview = 45 * time.Second
			awaiting = append(awaiting, item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list awaiting pull requests: %w", err)
	}

	return awaiting, nil
}

// GetFindingsBreakdown queries severity breakdown for a specific review from code_findings.
func (d *PostgresDashboardRepository) GetFindingsBreakdown(
	ctx context.Context,
	reviewID uuid.UUID,
) (criticals, majors, minors int, err error) {
	if d.repo == nil || d.repo.client == nil || d.repo.client.Pool == nil {
		return 0, 0, 0, errors.New("database repository unavailable")
	}

	query := `
		SELECT
			COUNT(*) FILTER (WHERE severity = 'CRITICAL')::int,
			COUNT(*) FILTER (WHERE severity IN ('HIGH', 'MAJOR'))::int,
			COUNT(*) FILTER (WHERE severity IN ('MEDIUM', 'LOW', 'MINOR'))::int
		FROM code_findings
		WHERE review_id = $1
	`

	err = d.repo.client.Pool.QueryRow(ctx, query, reviewID).Scan(
		&criticals,
		&majors,
		&minors,
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get findings breakdown: %w", err)
	}

	return criticals, majors, minors, nil
}
