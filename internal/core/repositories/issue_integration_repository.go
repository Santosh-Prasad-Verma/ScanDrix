package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgIssueRepository manages issues synchronized from Jira, Linear, and Azure Boards.
type PgIssueRepository struct {
	pool *pgxpool.Pool
}

// NewIssueRepository instantiates a new repository.
func NewIssueRepository(pool *pgxpool.Pool) *PgIssueRepository {
	return &PgIssueRepository{pool: pool}
}

// Create inserts a synchronized issue record.
func (r *PgIssueRepository) Create(ctx context.Context, issue *domain.Issue) error {
	query := `
		INSERT INTO issues (
			id, created_at, updated_at, workspace_id, repository_id, external_id,
			provider, title, description, severity, status, assignee, url, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	now := time.Now().UTC()
	if issue.ID == uuid.Nil {
		issue.ID = uuid.New()
	}
	issue.CreatedAt = now
	issue.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		issue.ID, issue.CreatedAt, issue.UpdatedAt, issue.WorkspaceID, issue.RepositoryID,
		issue.ExternalID, issue.Provider, issue.Title, issue.Description, issue.Severity,
		issue.Status, issue.Assignee, issue.URL, issue.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed creating issue: %w", err)
	}
	return nil
}

// FindByID retrieves an issue by its UUID within a workspace.
func (r *PgIssueRepository) FindByID(ctx context.Context, wsID, issueID uuid.UUID) (*domain.Issue, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, repository_id, external_id,
		       provider, title, description, severity, status, assignee, url, metadata
		FROM issues
		WHERE workspace_id = $1 AND id = $2
	`
	iss := &domain.Issue{}
	err := r.pool.QueryRow(ctx, query, wsID, issueID).Scan(
		&iss.ID, &iss.CreatedAt, &iss.UpdatedAt, &iss.WorkspaceID, &iss.RepositoryID,
		&iss.ExternalID, &iss.Provider, &iss.Title, &iss.Description, &iss.Severity,
		&iss.Status, &iss.Assignee, &iss.URL, &iss.Metadata,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying issue: %w", err)
	}
	return iss, nil
}

// ListByRepo returns all issues linked to a repository.
func (r *PgIssueRepository) ListByRepo(ctx context.Context, wsID, repoID uuid.UUID) ([]*domain.Issue, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, repository_id, external_id,
		       provider, title, description, severity, status, assignee, url, metadata
		FROM issues
		WHERE workspace_id = $1 AND repository_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, wsID, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed listing repository issues: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.Issue, 0)
	for rows.Next() {
		iss := &domain.Issue{}
		if err := rows.Scan(
			&iss.ID, &iss.CreatedAt, &iss.UpdatedAt, &iss.WorkspaceID, &iss.RepositoryID,
			&iss.ExternalID, &iss.Provider, &iss.Title, &iss.Description, &iss.Severity,
			&iss.Status, &iss.Assignee, &iss.URL, &iss.Metadata,
		); err != nil {
			return nil, fmt.Errorf("failed scanning issue row: %w", err)
		}
		items = append(items, iss)
	}
	return items, nil
}

// PgCodeReviewSettingsLogRepository records compliance audits for parameter changes.
type PgCodeReviewSettingsLogRepository struct {
	pool *pgxpool.Pool
}

// NewCodeReviewSettingsLogRepository instantiates a new repository.
func NewCodeReviewSettingsLogRepository(pool *pgxpool.Pool) *PgCodeReviewSettingsLogRepository {
	return &PgCodeReviewSettingsLogRepository{pool: pool}
}

// RecordChange inserts a configuration change audit record.
func (r *PgCodeReviewSettingsLogRepository) RecordChange(ctx context.Context, log *domain.CodeReviewSettingsLog) error {
	query := `
		INSERT INTO code_review_settings_logs (
			id, created_at, updated_at, workspace_id, modified_by_user_id,
			scope_type, scope_id, previous_values, updated_values, change_reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	now := time.Now().UTC()
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	log.CreatedAt = now
	log.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		log.ID, log.CreatedAt, log.UpdatedAt, log.WorkspaceID, log.ModifiedByUserID,
		log.ScopeType, log.ScopeID, log.PreviousValues, log.UpdatedValues, log.ChangeReason,
	)
	if err != nil {
		return fmt.Errorf("failed recording settings log: %w", err)
	}
	return nil
}
