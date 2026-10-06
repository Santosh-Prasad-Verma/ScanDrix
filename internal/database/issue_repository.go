// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: issue_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/issues"
	"github.com/scandrix/backend/pkg/models"
)

// CreateTrackedIssue persists a newly discovered vulnerability issue into PostgreSQL.
func (r *Repository) CreateTrackedIssue(ctx context.Context, issue *issues.TrackedIssue) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO tracked_issues (
			id, workspace_id, repository_id, title, description, file_path,
			start_line, end_line, severity, category, status, origin_review_id,
			remediation, fingerprint, external_issue_url, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	now := time.Now().UTC()
	if issue.ID == uuid.Nil {
		issue.ID = uuid.New()
	}
	issue.CreatedAt = now
	issue.UpdatedAt = now

	return r.client.ExecWithTenant(ctx, issue.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			issue.ID, issue.WorkspaceID, issue.RepositoryID, issue.Title, issue.Description,
			issue.FilePath, issue.StartLine, issue.EndLine, string(issue.Severity), issue.Category,
			string(issue.Status), issue.OriginReviewID, issue.Remediation, issue.Fingerprint,
			issue.ExternalIssueURL, issue.CreatedAt, issue.UpdatedAt,
		)
		return err
	})
}


// GetTrackedIssue retrieves an issue by its UUID scoped to tenant.
func (r *Repository) GetTrackedIssue(ctx context.Context, workspaceID, issueID uuid.UUID) (*issues.TrackedIssue, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, workspace_id, repository_id, title, description, file_path,
		       start_line, end_line, severity, category, status, origin_review_id,
		       remediation, fingerprint, external_issue_url, created_at, updated_at, resolved_at
		FROM tracked_issues
		WHERE workspace_id = $1 AND id = $2
	`
	var issue issues.TrackedIssue
	var sev string
	var stat string
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID, issueID).Scan(
			&issue.ID, &issue.WorkspaceID, &issue.RepositoryID, &issue.Title, &issue.Description,
			&issue.FilePath, &issue.StartLine, &issue.EndLine, &sev, &issue.Category,
			&stat, &issue.OriginReviewID, &issue.Remediation, &issue.Fingerprint,
			&issue.ExternalIssueURL, &issue.CreatedAt, &issue.UpdatedAt, &issue.ResolvedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	issue.Severity = models.FindingSeverity(sev)
	issue.Status = issues.IssueStatus(stat)
	return &issue, nil
}


// ListTrackedIssues returns issues for a workspace optionally filtered by status.
func (r *Repository) ListTrackedIssues(ctx context.Context, workspaceID uuid.UUID, status issues.IssueStatus) ([]issues.TrackedIssue, error) {
	if r == nil || r.client == nil {
		return []issues.TrackedIssue{}, nil
	}

	query := `
		SELECT id, workspace_id, repository_id, title, description, file_path,
		       start_line, end_line, severity, category, status, origin_review_id,
		       remediation, fingerprint, external_issue_url, created_at, updated_at, resolved_at
		FROM tracked_issues
		WHERE workspace_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
	`
	var result []issues.TrackedIssue
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID, string(status))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var issue issues.TrackedIssue
			var sev string
			var stat string
			if err := rows.Scan(
				&issue.ID, &issue.WorkspaceID, &issue.RepositoryID, &issue.Title, &issue.Description,
				&issue.FilePath, &issue.StartLine, &issue.EndLine, &sev, &issue.Category,
				&stat, &issue.OriginReviewID, &issue.Remediation, &issue.Fingerprint,
				&issue.ExternalIssueURL, &issue.CreatedAt, &issue.UpdatedAt, &issue.ResolvedAt,
			); err != nil {
				return err
			}
			issue.Severity = models.FindingSeverity(sev)
			issue.Status = issues.IssueStatus(stat)
			result = append(result, issue)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}


// CountTrackedIssues counts issues matching status filter.
func (r *Repository) CountTrackedIssues(ctx context.Context, workspaceID uuid.UUID, status issues.IssueStatus) (int, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}

	query := `
		SELECT COUNT(*)
		FROM tracked_issues
		WHERE workspace_id = $1 AND ($2 = '' OR status = $2)
	`
	var count int
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, workspaceID, string(status)).Scan(&count)
	})
	return count, err
}


// UpdateTrackedIssueStatus updates the status and resolved_at timestamp of an issue.
func (r *Repository) UpdateTrackedIssueStatus(ctx context.Context, workspaceID, issueID uuid.UUID, status issues.IssueStatus) error {
	if r == nil || r.client == nil {
		return nil
	}

	// resolved_at is cleared on any non-RESOLVED transition. Keeping the old
	// timestamp left a reopened finding looking resolved, so time-to-resolution
	// metrics measured from a stale closure instead of the current one.
	query := `
		UPDATE tracked_issues
		SET status = $1, updated_at = $2::timestamptz, resolved_at = CASE WHEN $1 = 'RESOLVED' THEN $2::timestamptz ELSE NULL END
		WHERE workspace_id = $3 AND id = $4
	`
	now := time.Now().UTC()
	return r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, string(status), now, workspaceID, issueID)
		return err
	})
}


// FindingTicketRecord models a ticket exported to Jira, Linear, or Azure Boards.
type FindingTicketRecord struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	FindingID   uuid.UUID `json:"finding_id"`
	Platform    string    `json:"platform"`
	TicketKey   string    `json:"ticket_key"`
	TicketURL   string    `json:"ticket_url"`
	CreatedAt   time.Time `json:"created_at"`
}


// InsertFindingTicket links a discovered security finding to an external PM ticket.
func (r *Repository) InsertFindingTicket(ctx context.Context, wsID, findingID uuid.UUID, platform, ticketKey, ticketURL string) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO finding_tickets (id, workspace_id, finding_id, platform, ticket_key, ticket_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (workspace_id, finding_id, platform) DO UPDATE
		SET ticket_key = EXCLUDED.ticket_key, ticket_url = EXCLUDED.ticket_url, created_at = NOW();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, findingID, platform, ticketKey, ticketURL)
		return err
	})
}


// GetTicketsForFinding retrieves external PM tickets linked to an individual finding.
func (r *Repository) GetTicketsForFinding(ctx context.Context, wsID, findingID uuid.UUID) ([]FindingTicketRecord, error) {
	if r == nil || r.client == nil {
		return []FindingTicketRecord{}, nil
	}

	query := `
		SELECT id, workspace_id, finding_id, platform, ticket_key, ticket_url, created_at
		FROM finding_tickets
		WHERE workspace_id = $1 AND finding_id = $2
		ORDER BY created_at DESC;
	`
	var list []FindingTicketRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, findingID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rec FindingTicketRecord
			if err := rows.Scan(&rec.ID, &rec.WorkspaceID, &rec.FindingID, &rec.Platform, &rec.TicketKey, &rec.TicketURL, &rec.CreatedAt); err != nil {
				return err
			}
			list = append(list, rec)
		}
		return rows.Err()
	})
	return list, err
}


// GetTicketsForReview retrieves all external PM tickets linked to findings under a review.
func (r *Repository) GetTicketsForReview(ctx context.Context, wsID, reviewID uuid.UUID) ([]FindingTicketRecord, error) {
	if r == nil || r.client == nil {
		return []FindingTicketRecord{}, nil
	}

	query := `
		SELECT t.id, t.workspace_id, t.finding_id, t.platform, t.ticket_key, t.ticket_url, t.created_at
		FROM finding_tickets t
		JOIN code_findings f ON f.id = t.finding_id AND f.workspace_id = t.workspace_id
		WHERE t.workspace_id = $1 AND f.review_id = $2
		ORDER BY t.created_at DESC;
	`
	var list []FindingTicketRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, reviewID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rec FindingTicketRecord
			if err := rows.Scan(&rec.ID, &rec.WorkspaceID, &rec.FindingID, &rec.Platform, &rec.TicketKey, &rec.TicketURL, &rec.CreatedAt); err != nil {
				return err
			}
			list = append(list, rec)
		}
		return rows.Err()
	})
	return list, err
}


// PMAutoTicketConfig stores auto-ticketing preferences per repository or workspace.
type PMAutoTicketConfig struct {
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	RepositoryID uuid.UUID `json:"repository_id"`
	Enabled      bool      `json:"enabled"`
	Platform     string    `json:"platform"`
	ProjectKey   string    `json:"project_key"`
	IssueType    string    `json:"issue_type"`
	MinSeverity  string    `json:"min_severity"`
}


// GetPMAutoTicketConfig retrieves auto-ticketing preferences for a repository.
func (r *Repository) GetPMAutoTicketConfig(ctx context.Context, wsID, repoID uuid.UUID) (*PMAutoTicketConfig, error) {
	if r == nil || r.client == nil {
		return &PMAutoTicketConfig{
			WorkspaceID:  wsID,
			RepositoryID: repoID,
			Enabled:      false,
			MinSeverity:  "HIGH",
		}, nil
	}

	query := `
		SELECT workspace_id, repository_id, enabled, platform, project_key, issue_type, min_severity
		FROM pm_auto_ticket_configs
		WHERE workspace_id = $1 AND repository_id = $2
		LIMIT 1;
	`
	var cfg PMAutoTicketConfig
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, repoID).Scan(
			&cfg.WorkspaceID, &cfg.RepositoryID, &cfg.Enabled, &cfg.Platform,
			&cfg.ProjectKey, &cfg.IssueType, &cfg.MinSeverity,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &PMAutoTicketConfig{
				WorkspaceID:  wsID,
				RepositoryID: repoID,
				Enabled:      false,
				MinSeverity:  "HIGH",
			}, nil
		}
		return nil, err
	}
	return &cfg, nil
}


// SetPMAutoTicketConfig configures automatic issue generation for a repository.
func (r *Repository) SetPMAutoTicketConfig(ctx context.Context, cfg PMAutoTicketConfig) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO pm_auto_ticket_configs (workspace_id, repository_id, enabled, platform, project_key, issue_type, min_severity, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (workspace_id, repository_id) DO UPDATE
		SET enabled = EXCLUDED.enabled, platform = EXCLUDED.platform, project_key = EXCLUDED.project_key,
		    issue_type = EXCLUDED.issue_type, min_severity = EXCLUDED.min_severity, updated_at = NOW();
	`
	return r.client.ExecWithTenant(ctx, cfg.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, cfg.WorkspaceID, cfg.RepositoryID, cfg.Enabled, cfg.Platform, cfg.ProjectKey, cfg.IssueType, cfg.MinSeverity)
		return err
	})
}

