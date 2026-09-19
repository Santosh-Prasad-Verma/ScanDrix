package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// PlatformPullRequestRepository defines database access for pull request records.
type PlatformPullRequestRepository interface {
	Upsert(ctx context.Context, pr *domain.PullRequest) error
	FindByNumber(ctx context.Context, wsID, repoID uuid.UUID, pullNumber int) (*domain.PullRequest, error)
	UpdateState(ctx context.Context, wsID, id uuid.UUID, state string) error
	ListOpen(ctx context.Context, wsID, repoID uuid.UUID) ([]*domain.PullRequest, error)
}

// PostgresPlatformPullRequestRepository implements PlatformPullRequestRepository.
type PostgresPlatformPullRequestRepository struct {
	db *sql.DB
}

// NewPostgresPlatformPullRequestRepository instantiates a platform PR repository.
func NewPostgresPlatformPullRequestRepository(db *sql.DB) *PostgresPlatformPullRequestRepository {
	return &PostgresPlatformPullRequestRepository{db: db}
}

// Upsert inserts or updates a pull request entity based on repository and pull number.
func (r *PostgresPlatformPullRequestRepository) Upsert(ctx context.Context, pr *domain.PullRequest) error {
	if pr.ID == uuid.Nil {
		pr.ID = uuid.New()
	}
	now := time.Now().UTC()
	pr.CreatedAt = now
	pr.UpdatedAt = now

	query := `
		INSERT INTO platform_pull_requests (
			id, workspace_id, repository_id, pull_number, title, author_username,
			author_avatar_url, source_branch, target_branch, head_sha, base_sha,
			state, is_draft, changed_files_count, additions, deletions,
			review_requested_at, merged_at, closed_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
		ON CONFLICT (workspace_id, repository_id, pull_number) DO UPDATE SET
			title = EXCLUDED.title,
			author_avatar_url = EXCLUDED.author_avatar_url,
			source_branch = EXCLUDED.source_branch,
			target_branch = EXCLUDED.target_branch,
			head_sha = EXCLUDED.head_sha,
			base_sha = EXCLUDED.base_sha,
			state = EXCLUDED.state,
			is_draft = EXCLUDED.is_draft,
			changed_files_count = EXCLUDED.changed_files_count,
			additions = EXCLUDED.additions,
			deletions = EXCLUDED.deletions,
			review_requested_at = COALESCE(EXCLUDED.review_requested_at, platform_pull_requests.review_requested_at),
			merged_at = EXCLUDED.merged_at,
			closed_at = EXCLUDED.closed_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.db.ExecContext(ctx, query,
		pr.ID, pr.WorkspaceID, pr.RepositoryID, pr.PullNumber, pr.Title, pr.AuthorUsername,
		pr.AuthorAvatarURL, pr.SourceBranch, pr.TargetBranch, pr.HeadSHA, pr.BaseSHA,
		pr.State, pr.IsDraft, pr.ChangedFilesCount, pr.Additions, pr.Deletions,
		pr.ReviewRequestedAt, pr.MergedAt, pr.ClosedAt, pr.CreatedAt, pr.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed upserting platform pull request: %w", err)
	}
	return nil
}

// FindByNumber retrieves a PR by repository and pull number.
func (r *PostgresPlatformPullRequestRepository) FindByNumber(ctx context.Context, wsID, repoID uuid.UUID, pullNumber int) (*domain.PullRequest, error) {
	query := `
		SELECT id, workspace_id, repository_id, pull_number, title, author_username,
		       author_avatar_url, source_branch, target_branch, head_sha, base_sha,
		       state, is_draft, changed_files_count, additions, deletions,
		       review_requested_at, merged_at, closed_at, created_at, updated_at
		FROM platform_pull_requests
		WHERE workspace_id = $1 AND repository_id = $2 AND pull_number = $3
	`
	row := r.db.QueryRowContext(ctx, query, wsID, repoID, pullNumber)
	var pr domain.PullRequest
	err := row.Scan(
		&pr.ID, &pr.WorkspaceID, &pr.RepositoryID, &pr.PullNumber, &pr.Title, &pr.AuthorUsername,
		&pr.AuthorAvatarURL, &pr.SourceBranch, &pr.TargetBranch, &pr.HeadSHA, &pr.BaseSHA,
		&pr.State, &pr.IsDraft, &pr.ChangedFilesCount, &pr.Additions, &pr.Deletions,
		&pr.ReviewRequestedAt, &pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching platform PR #%d: %w", pullNumber, err)
	}
	return &pr, nil
}

// UpdateState updates PR status ("OPEN", "CLOSED", "MERGED").
func (r *PostgresPlatformPullRequestRepository) UpdateState(ctx context.Context, wsID, id uuid.UUID, state string) error {
	now := time.Now().UTC()
	query := `
		UPDATE platform_pull_requests
		SET state = $1, updated_at = $2
		WHERE workspace_id = $3 AND id = $4
	`
	_, err := r.db.ExecContext(ctx, query, state, now, wsID, id)
	return err
}

// ListOpen lists active open PRs for a repository.
func (r *PostgresPlatformPullRequestRepository) ListOpen(ctx context.Context, wsID, repoID uuid.UUID) ([]*domain.PullRequest, error) {
	query := `
		SELECT id, workspace_id, repository_id, pull_number, title, author_username,
		       author_avatar_url, source_branch, target_branch, head_sha, base_sha,
		       state, is_draft, changed_files_count, additions, deletions,
		       review_requested_at, merged_at, closed_at, created_at, updated_at
		FROM platform_pull_requests
		WHERE workspace_id = $1 AND repository_id = $2 AND state = 'OPEN'
		ORDER BY pull_number DESC
	`
	rows, err := r.db.QueryContext(ctx, query, wsID, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*domain.PullRequest
	for rows.Next() {
		var pr domain.PullRequest
		if err := rows.Scan(
			&pr.ID, &pr.WorkspaceID, &pr.RepositoryID, &pr.PullNumber, &pr.Title, &pr.AuthorUsername,
			&pr.AuthorAvatarURL, &pr.SourceBranch, &pr.TargetBranch, &pr.HeadSHA, &pr.BaseSHA,
			&pr.State, &pr.IsDraft, &pr.ChangedFilesCount, &pr.Additions, &pr.Deletions,
			&pr.ReviewRequestedAt, &pr.MergedAt, &pr.ClosedAt, &pr.CreatedAt, &pr.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &pr)
	}
	return results, rows.Err()
}
