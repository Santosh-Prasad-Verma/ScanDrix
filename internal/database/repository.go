package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

// Repository provides clean-room, parameterized database operations adhering to Master Rule 5.3.
type Repository struct {
	client *Client
}

// NewRepository initializes a new data access layer.
func NewRepository(client *Client) *Repository {
	return &Repository{client: client}
}

// CreateWorkspace inserts a new enterprise organization workspace.
func (r *Repository) CreateWorkspace(ctx context.Context, ws *models.Workspace) error {
	query := `
		INSERT INTO workspaces (id, slug, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	now := time.Now().UTC()
	ws.CreatedAt = now
	ws.UpdatedAt = now
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}

	_, err := r.client.Pool.Exec(ctx, query, ws.ID, ws.Slug, ws.Name, ws.Status, ws.CreatedAt, ws.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}
	return nil
}

// CreateReview records a new incoming pull request review lifecycle job.
func (r *Repository) CreateReview(ctx context.Context, rev *models.PullRequestReview) error {
	query := `
		INSERT INTO pull_request_reviews (
			id, workspace_id, repository_id, pull_number, title, head_sha, base_sha, author_username, state, findings_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	rev.CreatedAt = now
	if rev.ID == uuid.Nil {
		rev.ID = uuid.New()
	}

	return r.client.ExecWithTenant(ctx, rev.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			rev.ID, rev.WorkspaceID, rev.RepositoryID, rev.PullNumber, rev.Title,
			rev.HeadSHA, rev.BaseSHA, rev.AuthorUsername, rev.State, rev.FindingsCount, rev.CreatedAt,
		)
		return err
	})
}

// UpdateReviewState updates the state and completion timestamp of a review job.
func (r *Repository) UpdateReviewState(ctx context.Context, tenantID, reviewID uuid.UUID, state models.ReviewState, findingsCount int) error {
	query := `
		UPDATE pull_request_reviews
		SET state = $1, findings_count = $2, completed_at = $3
		WHERE id = $4
	`
	now := time.Now().UTC()
	return r.client.ExecWithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, state, findingsCount, now, reviewID)
		return err
	})
}

// InsertOutboxEvent records an event inside the transactional boundary for guaranteed delivery (Outbox Pattern).
func (r *Repository) InsertOutboxEvent(ctx context.Context, event *models.OutboxRecord) error {
	query := `
		INSERT INTO outbox_events (id, workspace_id, event_type, payload, status, retry_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	now := time.Now().UTC()
	event.CreatedAt = now
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	return r.client.ExecWithTenant(ctx, event.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			event.ID, event.WorkspaceID, event.EventType, event.Payload,
			models.OutboxPending, 0, event.CreatedAt,
		)
		return err
	})
}

// FetchPendingOutboxEvents retrieves un-dispatched events for the background publisher relay.
func (r *Repository) FetchPendingOutboxEvents(ctx context.Context, limit int) ([]models.OutboxRecord, error) {
	query := `
		SELECT id, workspace_id, event_type, payload, status, retry_count, created_at
		FROM outbox_events
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`
	rows, err := r.client.Pool.Query(ctx, query, models.OutboxPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending outbox events: %w", err)
	}
	defer rows.Close()

	var records []models.OutboxRecord
	for rows.Next() {
		var rec models.OutboxRecord
		if err := rows.Scan(
			&rec.ID, &rec.WorkspaceID, &rec.EventType, &rec.Payload,
			&rec.Status, &rec.RetryCount, &rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("error scanning outbox row: %w", err)
		}
		records = append(records, rec)
	}
	return records, nil
}

// MarkOutboxEventPublished marks an event as successfully published to the message broker.
func (r *Repository) MarkOutboxEventPublished(ctx context.Context, eventID uuid.UUID) error {
	query := `
		UPDATE outbox_events
		SET status = $1, published_at = $2
		WHERE id = $3
	`
	now := time.Now().UTC()
	_, err := r.client.Pool.Exec(ctx, query, models.OutboxPublished, now, eventID)
	return err
}

// BatchInsertFindings persists actionable review findings discovered by the analysis pipeline.
func (r *Repository) BatchInsertFindings(ctx context.Context, tenantID uuid.UUID, findings []models.CodeFinding) error {
	if len(findings) == 0 {
		return nil
	}

	query := `
		INSERT INTO code_findings (
			id, review_id, workspace_id, file_path, start_line, end_line,
			severity, category, title, description, remediation, suggested_diff, fingerprint, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	return r.client.ExecWithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		for _, f := range findings {
			now := time.Now().UTC()
			fid := f.ID
			if fid == uuid.Nil {
				fid = uuid.New()
			}
			_, err := tx.Exec(ctx, query,
				fid, f.ReviewID, f.WorkspaceID, f.FilePath, f.StartLine, f.EndLine,
				f.Severity, f.Category, f.Title, f.Description, f.Remediation, f.SuggestedDiff, f.Fingerprint, now,
			)
			if err != nil {
				return fmt.Errorf("failed inserting finding %s: %w", fid, err)
			}
		}
		return nil
	})
}

// GetReviewFindings fetches all findings discovered for a specific pull request review.
func (r *Repository) GetReviewFindings(ctx context.Context, reviewID uuid.UUID) ([]models.CodeFinding, error) {
	if r == nil || r.client == nil {
		return []models.CodeFinding{}, nil
	}

	query := `
		SELECT id, review_id, workspace_id, file_path, start_line, end_line,
		       severity, category, title, description, remediation, suggested_diff, fingerprint
		FROM code_findings
		WHERE review_id = $1
		ORDER BY start_line ASC
	`
	rows, err := r.client.Pool.Query(ctx, query, reviewID)
	if err != nil {
		return nil, fmt.Errorf("failed querying findings: %w", err)
	}
	defer rows.Close()

	findings := make([]models.CodeFinding, 0)
	for rows.Next() {
		var f models.CodeFinding
		if err := rows.Scan(
			&f.ID, &f.ReviewID, &f.WorkspaceID, &f.FilePath, &f.StartLine, &f.EndLine,
			&f.Severity, &f.Category, &f.Title, &f.Description, &f.Remediation, &f.SuggestedDiff, &f.Fingerprint,
		); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, nil
}

