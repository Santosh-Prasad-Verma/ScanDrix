package database

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"github.com/scandrix/backend/internal/auth/cliauth"
	"github.com/scandrix/backend/internal/automation"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
	"github.com/scandrix/backend/internal/issues"
	"github.com/scandrix/backend/internal/provenance/intoto"
	"github.com/scandrix/backend/pkg/crypto"
	"github.com/scandrix/backend/pkg/models"
)

// tokenEncryptionKey is derived from the INTEGRATION_ENCRYPTION_KEY env var
// (or KMS_MASTER_KEY as fallback). It must NEVER be hardcoded in source.
// Generate with: openssl rand -hex 32
var tokenEncryptionKey [32]byte

func init() {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../../../.env")
	raw := os.Getenv("INTEGRATION_ENCRYPTION_KEY")
	if raw == "" {
		raw = os.Getenv("KMS_MASTER_KEY")
	}
	if raw == "" {
		log.Println("WARNING: INTEGRATION_ENCRYPTION_KEY and KMS_MASTER_KEY are not set. " +
			"Integration token encryption will fail at runtime. " +
			"Set one of these env vars (see .env.example).")
		// Use a zero key so the app can still boot for initial setup,
		// but any encrypt/decrypt call will produce a clear error.
		return
	}
	tokenEncryptionKey = sha256.Sum256([]byte(raw))
}

// Repository provides clean-room, parameterized database operations adhering to Master Rule 5.3.
type Repository struct {
	client *Client
}

// NewRepository initializes a new data access layer.
func NewRepository(client *Client) *Repository {
	return &Repository{client: client}
}

// GetWorkspaceIDByRepoNamespace resolves the owning workspace for a tracked
// repository by its provider and namespace path (e.g. "owner/repo"). Returns
// uuid.Nil if no matching active repository is found.
func (r *Repository) GetWorkspaceIDByRepoNamespace(ctx context.Context, provider, namespacePath string) (uuid.UUID, error) {
	row := r.client.Pool.QueryRow(ctx,
		`SELECT workspace_id FROM tracked_repositories
		 WHERE provider = $1 AND namespace_path = $2 AND is_active = TRUE
		 LIMIT 1`,
		provider, namespacePath)
	var wsID uuid.UUID
	if err := row.Scan(&wsID); err != nil {
		return uuid.Nil, fmt.Errorf("no active workspace for %s/%s: %w", provider, namespacePath, err)
	}
	return wsID, nil
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

// GetWorkspaceByID retrieves a workspace by its unique identifier.
func (r *Repository) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (*models.Workspace, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	query := `
		SELECT id, slug, name, status, created_at, updated_at
		FROM workspaces
		WHERE id = $1
	`
	var ws models.Workspace
	err := r.client.Pool.QueryRow(ctx, query, id).Scan(
		&ws.ID, &ws.Slug, &ws.Name, &ws.Status, &ws.CreatedAt, &ws.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ws, nil
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

// RetryDeadLetterOutboxEvents resets failed or DLQ outbox events back to PENDING status.
func (r *Repository) RetryDeadLetterOutboxEvents(ctx context.Context, limit int) (int, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}
	query := `
		UPDATE outbox_events
		SET status = 'PENDING', retry_count = 0, last_error = NULL
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE status IN ('FAILED', 'DEAD_LETTER', 'RETRYING')
			LIMIT $1
		)
	`
	cmdTag, err := r.client.Pool.Exec(ctx, query, limit)
	if err != nil {
		return 0, err
	}
	return int(cmdTag.RowsAffected()), nil
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

// GetReviewFindings fetches all findings discovered for a specific pull request review with tenant isolation.
func (r *Repository) GetReviewFindings(ctx context.Context, reviewID uuid.UUID, optionalWsID ...uuid.UUID) ([]models.CodeFinding, error) {
	if r == nil || r.client == nil {
		return []models.CodeFinding{}, nil
	}

	var findings []models.CodeFinding
	if len(optionalWsID) > 0 && optionalWsID[0] != uuid.Nil {
		wsID := optionalWsID[0]
		err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
			query := `
				SELECT id, review_id, workspace_id, file_path, start_line, end_line,
				       severity, category, title, description, remediation, suggested_diff, fingerprint
				FROM code_findings
				WHERE review_id = $1 AND workspace_id = $2
				ORDER BY start_line ASC
			`
			rows, err := tx.Query(ctx, query, reviewID, wsID)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var f models.CodeFinding
				if err := rows.Scan(
					&f.ID, &f.ReviewID, &f.WorkspaceID, &f.FilePath, &f.StartLine, &f.EndLine,
					&f.Severity, &f.Category, &f.Title, &f.Description, &f.Remediation, &f.SuggestedDiff, &f.Fingerprint,
				); err != nil {
					return err
				}
				findings = append(findings, f)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed querying findings with tenant: %w", err)
		}
		return findings, nil
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

	findings = make([]models.CodeFinding, 0)
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

// GetFindingByID retrieves an individual code finding by ID under tenant isolation.
func (r *Repository) GetFindingByID(ctx context.Context, wsID, findingID uuid.UUID) (*models.CodeFinding, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	var f models.CodeFinding
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		query := `
			SELECT id, review_id, workspace_id, file_path, start_line, end_line,
			       severity, category, title, description, remediation, suggested_diff, fingerprint, created_at
			FROM code_findings
			WHERE id = $1 AND workspace_id = $2
			LIMIT 1;
		`
		return tx.QueryRow(ctx, query, findingID, wsID).Scan(
			&f.ID, &f.ReviewID, &f.WorkspaceID, &f.FilePath, &f.StartLine, &f.EndLine,
			&f.Severity, &f.Category, &f.Title, &f.Description, &f.Remediation, &f.SuggestedDiff, &f.Fingerprint, &f.CreatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// GetReview fetches a single review by workspace and review ID.
func (r *Repository) GetReview(ctx context.Context, wsID, reviewID uuid.UUID) (*models.PullRequestReview, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	var rev models.PullRequestReview
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		query := `
			SELECT id, workspace_id, repository_id, pull_number, title,
			       head_sha, base_sha, author_username, state, findings_count,
			       created_at, completed_at
			FROM pull_request_reviews
			WHERE id = $1 AND workspace_id = $2
		`
		return tx.QueryRow(ctx, query, reviewID, wsID).Scan(
			&rev.ID, &rev.WorkspaceID, &rev.RepositoryID, &rev.PullNumber, &rev.Title,
			&rev.HeadSHA, &rev.BaseSHA, &rev.AuthorUsername, &rev.State, &rev.FindingsCount,
			&rev.CreatedAt, &rev.CompletedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rev, nil
}

// DismissFinding updates a finding status and records reason.
func (r *Repository) DismissFinding(ctx context.Context, wsID, findingID uuid.UUID, reason string) error {
	if r == nil || r.client == nil {
		return nil
	}
	query := `
		UPDATE code_findings
		SET remediation = $1
		WHERE id = $2 AND workspace_id = $3
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, "DISMISSED: "+reason, findingID, wsID)
		return err
	})
}

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

	query := `
		UPDATE tracked_issues
		SET status = $1, updated_at = $2, resolved_at = CASE WHEN $1 = 'RESOLVED' THEN $2 ELSE resolved_at END
		WHERE workspace_id = $3 AND id = $4
	`
	now := time.Now().UTC()
	return r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, string(status), now, workspaceID, issueID)
		return err
	})
}

// CreateAutomationRule registers a new automation workflow in PostgreSQL.
func (r *Repository) CreateAutomationRule(ctx context.Context, rule *automation.AutomationRule) error {
	if r == nil || r.client == nil {
		return nil
	}

	condJSON, _ := json.Marshal(rule.Conditions)
	actJSON, _ := json.Marshal(rule.Actions)

	query := `
		INSERT INTO workflow_automations (id, workspace_id, name, enabled, trigger, conditions, actions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now().UTC()
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now

	return r.client.ExecWithTenant(ctx, rule.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			rule.ID, rule.WorkspaceID, rule.Name, rule.Enabled, string(rule.Trigger),
			condJSON, actJSON, rule.CreatedAt, rule.UpdatedAt,
		)
		return err
	})
}

// ListAutomationRules retrieves all configured automation rules for a workspace.
func (r *Repository) ListAutomationRules(ctx context.Context, workspaceID uuid.UUID) ([]automation.AutomationRule, error) {
	if r == nil || r.client == nil {
		return []automation.AutomationRule{}, nil
	}

	query := `
		SELECT id, workspace_id, name, enabled, trigger, conditions, actions, created_at, updated_at
		FROM workflow_automations
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`
	var list []automation.AutomationRule
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rule automation.AutomationRule
			var trg string
			var condJSON, actJSON []byte
			if err := rows.Scan(
				&rule.ID, &rule.WorkspaceID, &rule.Name, &rule.Enabled, &trg,
				&condJSON, &actJSON, &rule.CreatedAt, &rule.UpdatedAt,
			); err != nil {
				return err
			}
			rule.Trigger = automation.TriggerType(trg)
			_ = json.Unmarshal(condJSON, &rule.Conditions)
			_ = json.Unmarshal(actJSON, &rule.Actions)
			list = append(list, rule)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// DeleteAutomationRule removes an automation rule.
func (r *Repository) DeleteAutomationRule(ctx context.Context, workspaceID, ruleID uuid.UUID) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		DELETE FROM workflow_automations
		WHERE workspace_id = $1 AND id = $2
	`
	return r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, workspaceID, ruleID)
		return err
	})
}

// UserRecord mirrors the users entity in the PostgreSQL database.
type UserRecord struct {
	UUID           uuid.UUID
	Email          string
	Password       string
	Role           string
	Status         string
	OrganizationID *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RefreshTokenRecord mirrors the auth entity in the PostgreSQL database.
type RefreshTokenRecord struct {
	UUID         uuid.UUID
	RefreshToken string
	ExpiryDate   time.Time
	Used         bool
	AuthProvider string
	UserUUID     uuid.UUID
}

// GetUserByEmail fetches a user record by email.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, email, password, role, status, organization_id, "createdAt", "updatedAt"
		FROM users
		WHERE LOWER(email) = LOWER($1)
		LIMIT 1;
	`
	var u UserRecord
	err := r.client.Pool.QueryRow(ctx, query, email).Scan(
		&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID fetches a user record by primary UUID.
func (r *Repository) GetUserByID(ctx context.Context, userUUID uuid.UUID) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, email, password, role, status, organization_id, "createdAt", "updatedAt"
		FROM users
		WHERE uuid = $1
		LIMIT 1;
	`
	var u UserRecord
	err := r.client.Pool.QueryRow(ctx, query, userUUID).Scan(
		&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateUser persists a new user with bcrypt password into the users table.
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash, role string, orgID *uuid.UUID) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		INSERT INTO users (uuid, email, password, role, status, organization_id, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4::users_role_enum, 'active', $5, now(), now())
		RETURNING uuid, email, password, role, status, organization_id, "createdAt", "updatedAt";
	`
	newID := uuid.New()
	var u UserRecord
	err := r.client.Pool.QueryRow(ctx, query, newID, email, passwordHash, role, orgID).Scan(
		&u.UUID, &u.Email, &u.Password, &u.Role, &u.Status, &u.OrganizationID, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// CreateRefreshToken records an active refresh token in the auth table.
func (r *Repository) CreateRefreshToken(ctx context.Context, userUUID uuid.UUID, refreshToken string, expiry time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		INSERT INTO auth (uuid, "userUuid", "refreshToken", "expiryDate", used, "authProvider", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, false, 'credentials', now(), now());
	`
	_, err := r.client.Pool.Exec(ctx, query, uuid.New(), userUUID, refreshToken, expiry)
	return err
}

// GetRefreshToken retrieves a refresh token record.
func (r *Repository) GetRefreshToken(ctx context.Context, refreshToken string) (*RefreshTokenRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, "refreshToken", "expiryDate", used, "authProvider", "userUuid"
		FROM auth
		WHERE "refreshToken" = $1
		LIMIT 1;
	`
	var t RefreshTokenRecord
	err := r.client.Pool.QueryRow(ctx, query, refreshToken).Scan(
		&t.UUID, &t.RefreshToken, &t.ExpiryDate, &t.Used, &t.AuthProvider, &t.UserUUID,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// MarkRefreshTokenUsed invalidates a refresh token as part of one-time token rotation (Master Rule 5.1).
func (r *Repository) MarkRefreshTokenUsed(ctx context.Context, refreshToken string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE auth
		SET used = true, "updatedAt" = now()
		WHERE "refreshToken" = $1;
	`
	_, err := r.client.Pool.Exec(ctx, query, refreshToken)
	return err
}

// SaveAPIKey records an issued CLI API key matching Kodus team_cli_key storage.
func (r *Repository) SaveAPIKey(ctx context.Context, id, workspaceID uuid.UUID, name, keyHash, prefix string, expiresAt *time.Time) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		INSERT INTO team_cli_key (uuid, name, "keyHash", active, "keyPrefix", config, "createdAt", "updatedAt")
		VALUES ($1, $2, $3, true, $4, '{}'::jsonb, now(), now());
	`
	_, err := r.client.Pool.Exec(ctx, query, id, name, keyHash, prefix)
	return err
}

// BatchInsertASTNodes persists extracted AST code symbols into the code_ast_nodes table (Migration 003).
func (r *Repository) BatchInsertASTNodes(ctx context.Context, repoID uuid.UUID, nodes []graph.ASTNode) error {
	if r == nil || r.client == nil || r.client.Pool == nil || len(nodes) == 0 {
		return nil
	}

	query := `
		INSERT INTO code_ast_nodes (
			id, repository_id, kind, symbol_name, file_path, start_line, end_line, signature, language, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			symbol_name = EXCLUDED.symbol_name,
			file_path = EXCLUDED.file_path,
			start_line = EXCLUDED.start_line,
			end_line = EXCLUDED.end_line,
			signature = EXCLUDED.signature,
			updated_at = EXCLUDED.updated_at;
	`

	batch := &pgx.Batch{}
	for _, n := range nodes {
		id := n.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		updatedAt := n.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}
		batch.Queue(query, id, repoID, string(n.Kind), n.SymbolName, n.FilePath, n.StartLine, n.EndLine, n.Signature, n.Language, updatedAt)
	}

	br := r.client.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range nodes {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to insert AST node batch: %w", err)
		}
	}
	return nil
}

// BatchInsertASTEdges persists architectural call and import graphs into code_ast_edges (Migration 003).
func (r *Repository) BatchInsertASTEdges(ctx context.Context, repoID uuid.UUID, edges []graph.ASTEdge) error {
	if r == nil || r.client == nil || r.client.Pool == nil || len(edges) == 0 {
		return nil
	}

	query := `
		INSERT INTO code_ast_edges (id, repository_id, from_node_id, to_node_id, kind)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING;
	`

	batch := &pgx.Batch{}
	for _, e := range edges {
		id := e.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		batch.Queue(query, id, repoID, e.FromNodeID, e.ToNodeID, string(e.Kind))
	}

	br := r.client.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range edges {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to insert AST edge batch: %w", err)
		}
	}
	return nil
}

// GetASTNodesByRepository retrieves indexed code symbols for a repository.
func (r *Repository) GetASTNodesByRepository(ctx context.Context, repoID uuid.UUID) ([]graph.ASTNode, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT id, repository_id, kind, symbol_name, file_path, start_line, end_line, signature, language, updated_at
		FROM code_ast_nodes
		WHERE repository_id = $1
		ORDER BY file_path, start_line;
	`

	rows, err := r.client.Pool.Query(ctx, query, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed querying AST nodes: %w", err)
	}
	defer rows.Close()

	var nodes []graph.ASTNode
	for rows.Next() {
		var n graph.ASTNode
		var kind string
		if err := rows.Scan(
			&n.ID, &n.RepositoryID, &kind, &n.SymbolName, &n.FilePath,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Language, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.Kind = graph.ASTNodeKind(kind)
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// GetASTCallers traverses code_ast_edges to locate functions calling targetNodeID.
func (r *Repository) GetASTCallers(ctx context.Context, repoID, targetNodeID uuid.UUID) ([]graph.ASTNode, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT n.id, n.repository_id, n.kind, n.symbol_name, n.file_path, n.start_line, n.end_line, n.signature, n.language, n.updated_at
		FROM code_ast_nodes n
		INNER JOIN code_ast_edges e ON e.from_node_id = n.id
		WHERE e.repository_id = $1 AND e.to_node_id = $2 AND e.kind = 'CALLS'
		ORDER BY n.file_path, n.start_line;
	`

	rows, err := r.client.Pool.Query(ctx, query, repoID, targetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed querying AST callers: %w", err)
	}
	defer rows.Close()

	var callers []graph.ASTNode
	for rows.Next() {
		var n graph.ASTNode
		var kind string
		if err := rows.Scan(
			&n.ID, &n.RepositoryID, &kind, &n.SymbolName, &n.FilePath,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Language, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.Kind = graph.ASTNodeKind(kind)
		callers = append(callers, n)
	}
	return callers, nil
}

// CreateCLISession stores an RFC 8628 terminal authorization session in PostgreSQL (cli_auth_sessions table).
func (r *Repository) CreateCLISession(ctx context.Context, s *cliauth.CLIDeviceSession) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		INSERT INTO cli_auth_sessions (
			uuid, state, device_code, user_code, redirect_uri, mode, status,
			expires_at, "createdAt", "updatedAt"
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.client.Pool.Exec(ctx, query,
		s.UUID, s.State, s.DeviceCode, s.UserCode, s.RedirectURI, s.Mode, string(s.Status),
		s.ExpiresAt, s.CreatedAt, s.UpdatedAt,
	)
	return err
}

// GetCLISessionByDeviceCode retrieves a session using its device_code.
func (r *Repository) GetCLISessionByDeviceCode(ctx context.Context, deviceCode string) (*cliauth.CLIDeviceSession, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT uuid, state, device_code, user_code, redirect_uri, mode, status,
		       access_token, refresh_token, user_id, user_email, user_agent,
		       expires_at, consumed_at, completed_at, "createdAt", "updatedAt"
		FROM cli_auth_sessions
		WHERE device_code = $1
		LIMIT 1;
	`
	var s cliauth.CLIDeviceSession
	var status string
	var access, refresh, email, agent *string
	var userID *uuid.UUID

	err := r.client.Pool.QueryRow(ctx, query, deviceCode).Scan(
		&s.UUID, &s.State, &s.DeviceCode, &s.UserCode, &s.RedirectURI, &s.Mode, &status,
		&access, &refresh, &userID, &email, &agent,
		&s.ExpiresAt, &s.ConsumedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Status = cliauth.SessionStatus(status)
	if access != nil {
		s.AccessToken = *access
	}
	if refresh != nil {
		s.RefreshToken = *refresh
	}
	if email != nil {
		s.UserEmail = *email
	}
	if agent != nil {
		s.UserAgent = *agent
	}
	s.UserID = userID
	return &s, nil
}

// GetCLISessionByUserCode retrieves a pending session using its 8-char user_code.
func (r *Repository) GetCLISessionByUserCode(ctx context.Context, userCode string) (*cliauth.CLIDeviceSession, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	query := `
		SELECT uuid, state, device_code, user_code, redirect_uri, mode, status,
		       access_token, refresh_token, user_id, user_email, user_agent,
		       expires_at, consumed_at, completed_at, "createdAt", "updatedAt"
		FROM cli_auth_sessions
		WHERE user_code = $1 AND status = 'pending'
		LIMIT 1;
	`
	var s cliauth.CLIDeviceSession
	var status string
	var access, refresh, email, agent *string
	var userID *uuid.UUID

	err := r.client.Pool.QueryRow(ctx, query, cleanCode).Scan(
		&s.UUID, &s.State, &s.DeviceCode, &s.UserCode, &s.RedirectURI, &s.Mode, &status,
		&access, &refresh, &userID, &email, &agent,
		&s.ExpiresAt, &s.ConsumedAt, &s.CompletedAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	s.Status = cliauth.SessionStatus(status)
	if access != nil {
		s.AccessToken = *access
	}
	if refresh != nil {
		s.RefreshToken = *refresh
	}
	if email != nil {
		s.UserEmail = *email
	}
	if agent != nil {
		s.UserAgent = *agent
	}
	s.UserID = userID
	return &s, nil
}

// CompleteCLISession marks a CLI auth session as completed with issued tokens.
func (r *Repository) CompleteCLISession(ctx context.Context, userCode, accessToken, refreshToken string, userID uuid.UUID, email string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(userCode))
	query := `
		UPDATE cli_auth_sessions
		SET status = 'completed', access_token = $1, refresh_token = $2,
		    user_id = $3, user_email = $4, completed_at = now(), "updatedAt" = now()
		WHERE user_code = $5 AND status = 'pending';
	`
	_, err := r.client.Pool.Exec(ctx, query, accessToken, refreshToken, userID, email, cleanCode)
	return err
}

// ConsumeCLISession marks an authorized session consumed so tokens cannot be replayed.
func (r *Repository) ConsumeCLISession(ctx context.Context, sessionID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE cli_auth_sessions
		SET status = 'consumed', consumed_at = now(), "updatedAt" = now()
		WHERE uuid = $1;
	`
	_, err := r.client.Pool.Exec(ctx, query, sessionID)
	return err
}

// UpdateUserPassword updates the bcrypt hash for an active user in the users table.
func (r *Repository) UpdateUserPassword(ctx context.Context, email, passwordHash string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE users
		SET password = $1, "updatedAt" = now()
		WHERE LOWER(email) = LOWER($2);
	`
	_, err := r.client.Pool.Exec(ctx, query, passwordHash, email)
	return err
}

// InvalidateAllUserRefreshTokens revokes all active refresh tokens for a user upon password reset.
func (r *Repository) InvalidateAllUserRefreshTokens(ctx context.Context, userUUID uuid.UUID) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE auth
		SET used = true, "updatedAt" = now()
		WHERE "userUuid" = $1;
	`
	_, err := r.client.Pool.Exec(ctx, query, userUUID)
	return err
}

// CreateTeam persists a new team under tenant isolation.
func (r *Repository) CreateTeam(ctx context.Context, wsID uuid.UUID, name, description string) (*models.Team, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	team := &models.Team{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	query := `
		INSERT INTO teams (id, workspace_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (workspace_id, name) DO UPDATE
		SET description = EXCLUDED.description, updated_at = EXCLUDED.updated_at
		RETURNING id, workspace_id, name, description, created_at, updated_at;
	`

	var res models.Team
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, team.ID, team.WorkspaceID, team.Name, team.Description, team.CreatedAt, team.UpdatedAt).
			Scan(&res.ID, &res.WorkspaceID, &res.Name, &res.Description, &res.CreatedAt, &res.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating team: %w", err)
	}
	return &res, nil
}

// ListTeams retrieves all teams belonging to the workspace.
func (r *Repository) ListTeams(ctx context.Context, wsID uuid.UUID) ([]models.Team, error) {
	if r == nil || r.client == nil {
		return []models.Team{}, nil
	}

	query := `
		SELECT id, workspace_id, name, description, created_at, updated_at
		FROM teams
		WHERE workspace_id = $1
		ORDER BY name ASC;
	`

	var teams []models.Team
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t models.Team
			if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.Description, &t.CreatedAt, &t.UpdatedAt); err != nil {
				return err
			}
			teams = append(teams, t)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing teams: %w", err)
	}
	return teams, nil
}

// AddTeamMember adds a user to a team.
func (r *Repository) AddTeamMember(ctx context.Context, teamID, userID uuid.UUID, email, role string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO team_members (id, team_id, user_id, email, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (team_id, user_id) DO UPDATE
		SET role = EXCLUDED.role, email = EXCLUDED.email;
	`
	_, err := r.client.Pool.Exec(ctx, query, uuid.New(), teamID, userID, email, role, time.Now().UTC())
	return err
}

// ListTeamMembers lists all members of a team.
func (r *Repository) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]models.TeamMember, error) {
	if r == nil || r.client == nil {
		return []models.TeamMember{}, nil
	}

	query := `
		SELECT id, team_id, user_id, email, role, created_at
		FROM team_members
		WHERE team_id = $1
		ORDER BY created_at ASC;
	`
	rows, err := r.client.Pool.Query(ctx, query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.TeamMember
	for rows.Next() {
		var m models.TeamMember
		if err := rows.Scan(&m.ID, &m.TeamID, &m.UserID, &m.Email, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// RemoveTeamMember removes a user from a team.
func (r *Repository) RemoveTeamMember(ctx context.Context, teamID, userID uuid.UUID) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2;`
	_, err := r.client.Pool.Exec(ctx, query, teamID, userID)
	return err
}

// ListTrackedRepositories lists all monitored repositories for a workspace.
func (r *Repository) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return []models.TrackedRepository{}, nil
	}

	query := `
		SELECT id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at
		FROM tracked_repositories
		WHERE workspace_id = $1
		ORDER BY namespace_path ASC;
	`

	var repos []models.TrackedRepository
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var repo models.TrackedRepository
			if err := rows.Scan(
				&repo.ID, &repo.WorkspaceID, &repo.Provider, &repo.ExternalID,
				&repo.NamespacePath, &repo.DefaultBranch, &repo.IsActive,
				&repo.CreatedAt, &repo.UpdatedAt,
			); err != nil {
				return err
			}
			repos = append(repos, repo)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing tracked repositories: %w", err)
	}
	return repos, nil
}

// TrackRepository registers or updates a monitored repository.
func (r *Repository) TrackRepository(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, externalID, namespacePath, defaultBranch string) (*models.TrackedRepository, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	if defaultBranch == "" {
		defaultBranch = "main"
	}
	repo := &models.TrackedRepository{
		ID:            uuid.New(),
		WorkspaceID:   wsID,
		Provider:      provider,
		ExternalID:    externalID,
		NamespacePath: namespacePath,
		DefaultBranch: defaultBranch,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	query := `
		INSERT INTO tracked_repositories (id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (workspace_id, provider, external_id) DO UPDATE
		SET namespace_path = EXCLUDED.namespace_path, default_branch = EXCLUDED.default_branch, is_active = EXCLUDED.is_active, updated_at = EXCLUDED.updated_at
		RETURNING id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at;
	`

	var res models.TrackedRepository
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			repo.ID, repo.WorkspaceID, string(repo.Provider), repo.ExternalID,
			repo.NamespacePath, repo.DefaultBranch, repo.IsActive, repo.CreatedAt, repo.UpdatedAt,
		).Scan(&res.ID, &res.WorkspaceID, &res.Provider, &res.ExternalID, &res.NamespacePath, &res.DefaultBranch, &res.IsActive, &res.CreatedAt, &res.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed tracking repository: %w", err)
	}
	return &res, nil
}

// GetWorkspaceParameters loads the review and organization parameters JSON payloads.
func (r *Repository) GetWorkspaceParameters(ctx context.Context, wsID uuid.UUID) (reviewParams, orgParams []byte, err error) {
	if r == nil || r.client == nil {
		return []byte("{}"), []byte("{}"), nil
	}

	query := `SELECT review_params, org_params FROM workspace_parameters WHERE workspace_id = $1;`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, query, wsID)
		return row.Scan(&reviewParams, &orgParams)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []byte("{}"), []byte("{}"), nil
		}
		return nil, nil, err
	}
	return reviewParams, orgParams, nil
}

// UpdateWorkspaceReviewParameters stores updated review settings.
func (r *Repository) UpdateWorkspaceReviewParameters(ctx context.Context, wsID uuid.UUID, reviewParams []byte) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO workspace_parameters (workspace_id, review_params, org_params, updated_at)
		VALUES ($1, $2, '{}'::jsonb, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET review_params = EXCLUDED.review_params, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, reviewParams)
		return err
	})
}

// UpdateWorkspaceOrgParameters stores updated organizational governance thresholds.
func (r *Repository) UpdateWorkspaceOrgParameters(ctx context.Context, wsID uuid.UUID, orgParams []byte) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO workspace_parameters (workspace_id, review_params, org_params, updated_at)
		VALUES ($1, '{}'::jsonb, $2, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET org_params = EXCLUDED.org_params, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, orgParams)
		return err
	})
}

// ListNotificationChannels retrieves configured alert destinations.
func (r *Repository) ListNotificationChannels(ctx context.Context, wsID uuid.UUID) ([]models.NotificationChannel, error) {
	if r == nil || r.client == nil {
		return []models.NotificationChannel{}, nil
	}

	query := `
		SELECT id, workspace_id, type, target, severity, enabled, created_at
		FROM notification_channels
		WHERE workspace_id = $1
		ORDER BY created_at ASC;
	`
	var channels []models.NotificationChannel
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ch models.NotificationChannel
			if err := rows.Scan(&ch.ID, &ch.WorkspaceID, &ch.Type, &ch.Target, &ch.Severity, &ch.Enabled, &ch.CreatedAt); err != nil {
				return err
			}
			channels = append(channels, ch)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing channels: %w", err)
	}
	return channels, nil
}

// CreateNotificationChannel creates a new alert destination.
func (r *Repository) CreateNotificationChannel(ctx context.Context, wsID uuid.UUID, chType, target string, severity models.FindingSeverity) (*models.NotificationChannel, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	ch := &models.NotificationChannel{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		Type:        chType,
		Target:      target,
		Severity:    severity,
		Enabled:     true,
		CreatedAt:   time.Now().UTC(),
	}

	query := `
		INSERT INTO notification_channels (id, workspace_id, type, target, severity, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, workspace_id, type, target, severity, enabled, created_at;
	`

	var res models.NotificationChannel
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, ch.ID, ch.WorkspaceID, ch.Type, ch.Target, string(ch.Severity), ch.Enabled, ch.CreatedAt).
			Scan(&res.ID, &res.WorkspaceID, &res.Type, &res.Target, &res.Severity, &res.Enabled, &res.CreatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating channel: %w", err)
	}
	return &res, nil
}

// ListIntegrationConnections retrieves external SCM connections for the workspace.
func (r *Repository) ListIntegrationConnections(ctx context.Context, wsID uuid.UUID) ([]models.IntegrationConnection, error) {
	if r == nil || r.client == nil {
		return []models.IntegrationConnection{}, nil
	}

	query := `
		SELECT id, workspace_id, provider, account_name, is_connected, access_token_enc, repo_count, last_synced_at, created_at, updated_at
		FROM integration_connections
		WHERE workspace_id = $1
		ORDER BY provider ASC;
	`

	var conns []models.IntegrationConnection
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c models.IntegrationConnection
			if err := rows.Scan(
				&c.ID, &c.WorkspaceID, &c.Provider, &c.AccountName, &c.IsConnected,
				&c.AccessTokenEnc, &c.RepoCount, &c.LastSyncedAt, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				return err
			}
			if c.AccessTokenEnc != "" {
				tenantKey := deriveTenantIntegrationKey(wsID)
				if dec, err := crypto.DecryptStringAESGCM(tenantKey, c.AccessTokenEnc); err == nil {
					c.AccessTokenEnc = dec
				} else if dec, err := crypto.DecryptStringAESGCM(tokenEncryptionKey[:], c.AccessTokenEnc); err == nil {
					c.AccessTokenEnc = dec
				}
			}
			conns = append(conns, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing integrations: %w", err)
	}
	return conns, nil
}

func deriveTenantIntegrationKey(wsID uuid.UUID) []byte {
	k := sha256.Sum256([]byte("scandrix_integration_kdf_v1:" + wsID.String()))
	return k[:]
}

// UpsertIntegrationConnection stores or updates an SCM connection with encrypted credentials.
func (r *Repository) UpsertIntegrationConnection(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider, accountName, tokenPlain string, isConnected bool, repoCount int) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	tokenEnc := tokenPlain
	if tokenPlain != "" {
		tenantKey := deriveTenantIntegrationKey(wsID)
		if enc, err := crypto.EncryptStringAESGCM(tenantKey, tokenPlain); err == nil {
			tokenEnc = enc
		}
	}

	query := `
		INSERT INTO integration_connections (id, workspace_id, provider, account_name, is_connected, access_token_enc, repo_count, last_synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now(), now())
		ON CONFLICT (workspace_id, provider) DO UPDATE
		SET account_name = EXCLUDED.account_name, is_connected = EXCLUDED.is_connected, access_token_enc = EXCLUDED.access_token_enc, repo_count = EXCLUDED.repo_count, last_synced_at = now(), updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, string(provider), accountName, isConnected, tokenEnc, repoCount)
		return err
	})
}

// GetDecryptedIntegrationToken retrieves and decrypts the stored integration token for a provider.
func (r *Repository) GetDecryptedIntegrationToken(ctx context.Context, wsID uuid.UUID, provider models.SCMProvider) (string, error) {
	if r == nil || r.client == nil {
		return "", nil
	}
	conns, err := r.ListIntegrationConnections(ctx, wsID)
	if err != nil {
		return "", err
	}
	for _, c := range conns {
		if c.Provider == provider && c.IsConnected && c.AccessTokenEnc != "" {
			tenantKey := deriveTenantIntegrationKey(wsID)
			if dec, err := crypto.DecryptStringAESGCM(tenantKey, c.AccessTokenEnc); err == nil && dec != "" {
				return dec, nil
			}
			return c.AccessTokenEnc, nil
		}
	}
	return "", nil
}

// RecordFindingFeedback records user feedback on a finding.
func (r *Repository) RecordFindingFeedback(ctx context.Context, wsID, findingID uuid.UUID, sentiment, comment string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO finding_feedback (id, workspace_id, findingID, sentiment, comment, created_at)
		VALUES ($1, $2, $3, $4, $5, now());
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, findingID, sentiment, comment)
		return err
	})
}

// InsertAuditLog records an immutable security event into PostgreSQL.
func (r *Repository) InsertAuditLog(ctx context.Context, wsID uuid.UUID, actorID, actorEmail, ipAddress, action, targetType, targetID string, metadata []byte) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	if len(metadata) == 0 {
		metadata = []byte("{}")
	}

	query := `
		INSERT INTO audit_logs (id, workspace_id, actor_id, actor_email, ip_address, action, target_type, target_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now());
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, actorID, actorEmail, ipAddress, action, targetType, targetID, metadata)
		return err
	})
}

// ListAuditLogs retrieves the latest audit log records for the workspace.
func (r *Repository) ListAuditLogs(ctx context.Context, wsID uuid.UUID, limit int) ([]models.AuditLogRecord, error) {
	if r == nil || r.client == nil {
		return []models.AuditLogRecord{}, nil
	}

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT id, workspace_id, actor_id, actor_email, ip_address, action, target_type, target_id, metadata, created_at
		FROM audit_logs
		WHERE workspace_id = $1
		ORDER BY created_at DESC
		LIMIT $2;
	`
	var logs []models.AuditLogRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var l models.AuditLogRecord
			if err := rows.Scan(
				&l.ID, &l.WorkspaceID, &l.ActorID, &l.ActorEmail, &l.IPAddress,
				&l.Action, &l.TargetType, &l.TargetID, &l.Metadata, &l.CreatedAt,
			); err != nil {
				return err
			}
			logs = append(logs, l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing audit logs: %w", err)
	}
	return logs, nil
}

// RecordTokenUsage logs token consumption for billing and usage caps.
func (r *Repository) RecordTokenUsage(ctx context.Context, wsID uuid.UUID, reviewID *uuid.UUID, promptTokens, completionTokens int64, costUSD float64) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO token_usage_records (id, workspace_id, review_id, prompt_tokens, completion_tokens, cost_usd, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now());
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, reviewID, promptTokens, completionTokens, costUSD)
		return err
	})
}

// GetWorkspaceUsage calculates aggregated token usage and cost for a given time window.
func (r *Repository) GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (promptTokens, completionTokens int64, costUSD float64, err error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, nil
	}

	query := `
		SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cost_usd), 0)
		FROM token_usage_records
		WHERE workspace_id = $1 AND created_at >= $2;
	`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID, since).Scan(&promptTokens, &completionTokens, &costUSD)
	})
	return promptTokens, completionTokens, costUSD, err
}

// UpdateSpendLimit updates the monthly spend limit for a workspace.
func (r *Repository) UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO workspace_spend_limits (workspace_id, monthly_spend_limit_usd, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (workspace_id) DO UPDATE
		SET monthly_spend_limit_usd = EXCLUDED.monthly_spend_limit_usd, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, wsID, limitUSD)
		return err
	})
}

// GetSpendLimit retrieves the configured monthly spend limit.
func (r *Repository) GetSpendLimit(ctx context.Context, wsID uuid.UUID) (float64, error) {
	if r == nil || r.client == nil {
		return 50.00, nil
	}

	query := `SELECT monthly_spend_limit_usd FROM workspace_spend_limits WHERE workspace_id = $1;`
	var limit float64
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&limit)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 50.00, nil
		}
		return 0, err
	}
	return limit, nil
}

// GetActiveLicense retrieves the active enterprise license for a workspace.
func (r *Repository) GetActiveLicense(ctx context.Context, wsID uuid.UUID) (*models.OrganizationLicense, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	query := `
		SELECT id, workspace_id, license_key, organization_name, plan_tier, total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled, activated_at, updated_at
		FROM organization_licenses
		WHERE workspace_id = $1
		ORDER BY activated_at DESC
		LIMIT 1;
	`
	var lic models.OrganizationLicense
	var featuresJSON []byte
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(
			&lic.ID, &lic.WorkspaceID, &lic.LicenseKey, &lic.OrganizationName,
			&lic.PlanTier, &lic.TotalSeats, &lic.AllocatedSeats, &lic.ExpiresAt,
			&lic.IsAirGapped, &featuresJSON, &lic.ActivatedAt, &lic.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(featuresJSON, &lic.FeaturesEnabled)
	return &lic, nil
}

// ActivateLicense stores or updates an enterprise license in PostgreSQL.
func (r *Repository) ActivateLicense(ctx context.Context, wsID uuid.UUID, licenseKey, orgName, planTier string, totalSeats int, expiresAt time.Time, features []string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	featuresJSON, _ := json.Marshal(features)
	query := `
		INSERT INTO organization_licenses (id, workspace_id, license_key, organization_name, plan_tier, total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled, activated_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, $7, false, $8, now(), now())
		ON CONFLICT (license_key) DO UPDATE
		SET total_seats = EXCLUDED.total_seats, expires_at = EXCLUDED.expires_at, features_enabled = EXCLUDED.features_enabled, updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, uuid.New(), wsID, licenseKey, orgName, planTier, totalSeats, expiresAt, featuresJSON)
		return err
	})
}

// GetOutboxMetrics queries delivery stats from outbox_events.
func (r *Repository) GetOutboxMetrics(ctx context.Context, wsID uuid.UUID) (totalDelivered, pending, retrying, dlq int64, lastEventAt *time.Time, err error) {
	if r == nil || r.client == nil {
		return 0, 0, 0, 0, nil, nil
	}

	query := `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE status = 'PUBLISHED'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'PENDING'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'PENDING' AND retry_count > 0), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = 'FAILED'), 0),
			MAX(created_at)
		FROM outbox_events
		WHERE workspace_id = $1;
	`
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(&totalDelivered, &pending, &retrying, &dlq, &lastEventAt)
	})
	return totalDelivered, pending, retrying, dlq, lastEventAt, err
}

// RecordBillingTransaction stores a checkout or webhook transaction in PostgreSQL.
func (r *Repository) RecordBillingTransaction(ctx context.Context, tx *models.BillingTransaction) error {
	if r == nil || r.client == nil {
		return nil
	}
	if tx.ID == uuid.Nil {
		tx.ID = uuid.New()
	}

	query := `
		INSERT INTO billing_transactions (
			id, workspace_id, provider, order_id, payment_id, signature,
			amount, currency, plan_tier, status, receipt, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			payment_id = EXCLUDED.payment_id,
			signature = EXCLUDED.signature,
			status = EXCLUDED.status,
			updated_at = now();
	`
	return r.client.ExecWithTenant(ctx, tx.WorkspaceID, func(pgTx pgx.Tx) error {
		_, err := pgTx.Exec(ctx, query,
			tx.ID, tx.WorkspaceID, tx.Provider, tx.OrderID, tx.PaymentID, tx.Signature,
			tx.Amount, tx.Currency, tx.PlanTier, tx.Status, tx.Receipt,
		)
		return err
	})
}

// UpdateBillingTransactionStatus updates the status and payment ID of an order.
func (r *Repository) UpdateBillingTransactionStatus(ctx context.Context, wsID uuid.UUID, orderID, paymentID, signature, status string) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		UPDATE billing_transactions
		SET payment_id = COALESCE(NULLIF($1, ''), payment_id),
		    signature = COALESCE(NULLIF($2, ''), signature),
		    status = $3,
		    updated_at = now()
		WHERE workspace_id = $4 AND order_id = $5;
	`
	return r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		_, err := pgTx.Exec(ctx, query, paymentID, signature, status, wsID, orderID)
		return err
	})
}

// UpgradeWorkspacePlan updates the active organization license and seat table to reflect a purchased plan.
func (r *Repository) UpgradeWorkspacePlan(ctx context.Context, wsID uuid.UUID, planTier string, maxSeats int, expiresAt time.Time, features []string) error {
	if r == nil || r.client == nil {
		return nil
	}

	featuresJSON, _ := json.Marshal(features)
	licenseKey := fmt.Sprintf("SUB-%s-%s", strings.ToUpper(planTier), wsID.String()[:8])

	queryLicense := `
		INSERT INTO organization_licenses (
			id, workspace_id, license_key, organization_name, plan_tier,
			total_seats, allocated_seats, expires_at, is_air_gapped, features_enabled,
			activated_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 1, $7, false, $8, now(), now())
		ON CONFLICT (license_key) DO UPDATE SET
			plan_tier = EXCLUDED.plan_tier,
			total_seats = EXCLUDED.total_seats,
			expires_at = EXCLUDED.expires_at,
			features_enabled = EXCLUDED.features_enabled,
			updated_at = now();
	`

	querySeats := `
		INSERT INTO organization_billing_seats (
			id, workspace_id, tier, max_seats, allocated_seats, byok_enabled, dora_enabled, active_until, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 1, true, true, $5, now(), now())
		ON CONFLICT (id) DO NOTHING;
	`

	return r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		if _, err := pgTx.Exec(ctx, queryLicense, uuid.New(), wsID, licenseKey, planTier+" Plan", planTier, maxSeats, expiresAt, featuresJSON); err != nil {
			return err
		}
		_, err := pgTx.Exec(ctx, querySeats, uuid.New(), wsID, planTier, maxSeats, expiresAt)
		return err
	})
}

// GetPlanConfiguration fetches dynamic plan pricing and quotas from PostgreSQL.
func (r *Repository) GetPlanConfiguration(ctx context.Context, tier string) (*models.PlanConfiguration, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}

	normTier := strings.ToUpper(strings.TrimSpace(tier))
	if normTier == "PRO" {
		normTier = "TEAM"
	}

	query := `
		SELECT tier, display_name, amount_inr, amount_usd, monthly_tokens,
		       burst_limit_per_min, max_seats, max_repositories, max_concurrent_reviews,
		       byok_allowed, allocated_models, features_enabled, created_at, updated_at
		FROM plan_configurations
		WHERE tier = $1;
	`
	var plan models.PlanConfiguration
	var modelsJSON, featuresJSON []byte

	err := r.client.Pool.QueryRow(ctx, query, normTier).Scan(
		&plan.Tier, &plan.DisplayName, &plan.AmountINR, &plan.AmountUSD,
		&plan.MonthlyTokens, &plan.BurstLimitPerMin, &plan.MaxSeats,
		&plan.MaxRepositories, &plan.MaxConcurrentReviews, &plan.BYOKAllowed,
		&modelsJSON, &featuresJSON, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	_ = json.Unmarshal(modelsJSON, &plan.AllocatedModels)
	_ = json.Unmarshal(featuresJSON, &plan.FeaturesEnabled)
	return &plan, nil
}

// ListPlanConfigurations returns all active subscription plans from PostgreSQL.
func (r *Repository) ListPlanConfigurations(ctx context.Context) ([]models.PlanConfiguration, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}

	query := `
		SELECT tier, display_name, amount_inr, amount_usd, monthly_tokens,
		       burst_limit_per_min, max_seats, max_repositories, max_concurrent_reviews,
		       byok_allowed, allocated_models, features_enabled, created_at, updated_at
		FROM plan_configurations
		ORDER BY amount_inr ASC;
	`
	rows, err := r.client.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []models.PlanConfiguration
	for rows.Next() {
		var plan models.PlanConfiguration
		var modelsJSON, featuresJSON []byte
		if err := rows.Scan(
			&plan.Tier, &plan.DisplayName, &plan.AmountINR, &plan.AmountUSD,
			&plan.MonthlyTokens, &plan.BurstLimitPerMin, &plan.MaxSeats,
			&plan.MaxRepositories, &plan.MaxConcurrentReviews, &plan.BYOKAllowed,
			&modelsJSON, &featuresJSON, &plan.CreatedAt, &plan.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(modelsJSON, &plan.AllocatedModels)
		_ = json.Unmarshal(featuresJSON, &plan.FeaturesEnabled)
		plans = append(plans, plan)
	}
	return plans, nil
}

// GetWorkspacePlanDetails queries the real license, database-configured quotas, and aggregated token consumption.
func (r *Repository) GetWorkspacePlanDetails(ctx context.Context, wsID uuid.UUID) (*models.WorkspacePlanDetails, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	// 1. Fetch active license
	lic, err := r.GetActiveLicense(ctx, wsID)
	if err != nil {
		return nil, err
	}

	activeTier := "COMMUNITY"
	orgName := "Community Workspace"
	totalSeats := 5
	allocatedSeats := 1
	expiresAt := time.Now().AddDate(10, 0, 0)
	features := []string{"automated_reviews", "custom_rules"}

	if lic != nil {
		activeTier = strings.ToUpper(lic.PlanTier)
		if activeTier == "PRO" {
			activeTier = "TEAM"
		}
		orgName = lic.OrganizationName
		totalSeats = lic.TotalSeats
		allocatedSeats = lic.AllocatedSeats
		expiresAt = lic.ExpiresAt
		features = lic.FeaturesEnabled
	}

	// 2. Fetch database plan configuration (DB as Single Source of Truth)
	planConfig, _ := r.GetPlanConfiguration(ctx, activeTier)
	monthlyLimit := int64(500000)
	burstLimit := int64(50000)
	var allocatedModels []string

	if planConfig != nil {
		monthlyLimit = planConfig.MonthlyTokens
		burstLimit = planConfig.BurstLimitPerMin
		allocatedModels = planConfig.AllocatedModels
		if len(planConfig.FeaturesEnabled) > 0 {
			features = planConfig.FeaturesEnabled
		}
	} else {
		allocatedModels = []string{
			"gemini-2.5-flash-lite", "gemini-3.1-flash-lite", "gemini-2.5-flash",
			"minimax/minimax-m3:free", "stealth/ox-alpha", "thinkingmachines/inkling:free",
		}
	}

	// 3. Query real token consumption from PostgreSQL token_usage_records table
	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	startOfMinute := time.Now().Add(-1 * time.Minute)

	promptMonth, compMonth, _, _ := r.GetWorkspaceUsage(ctx, wsID, startOfMonth)
	promptMin, compMin, _, _ := r.GetWorkspaceUsage(ctx, wsID, startOfMinute)

	return &models.WorkspacePlanDetails{
		WorkspaceID:       wsID,
		PlanTier:          activeTier,
		OrganizationName:  orgName,
		TotalSeats:        totalSeats,
		AllocatedSeats:    allocatedSeats,
		ExpiresAt:         expiresAt,
		MonthlyTokenLimit: monthlyLimit,
		MonthlyTokensUsed: promptMonth + compMonth,
		BurstLimitPerMin:  burstLimit,
		BurstTokensUsed:   promptMin + compMin,
		AllocatedModels:   allocatedModels,
		FeaturesEnabled:   features,
		BYOKAllowed:       true,
	}, nil
}

// GetWorkspaceOwner retrieves the email and display name of the workspace owner or admin for billing notifications.
func (r *Repository) GetWorkspaceOwner(ctx context.Context, wsID uuid.UUID) (email, displayName string, err error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return "", "", nil
	}

	query := `
		SELECT email, display_name
		FROM account_profiles
		WHERE workspace_id = $1
		ORDER BY CASE WHEN UPPER(role) = 'OWNER' THEN 1 WHEN UPPER(role) = 'ADMIN' THEN 2 ELSE 3 END, created_at ASC
		LIMIT 1;
	`
	err = r.client.Pool.QueryRow(ctx, query, wsID).Scan(&email, &displayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", err
	}
	return email, displayName, nil
}

// GetCockpitMetrics aggregates real review, finding, repository, and developer stats for a workspace.
func (r *Repository) GetCockpitMetrics(ctx context.Context, wsID uuid.UUID) (*models.CockpitMetrics, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return &models.CockpitMetrics{
			PassRatePercentage: 100.0,
		}, nil
	}

	query := `
		SELECT 
			COUNT(DISTINCT r.id) AS total_reviews,
			COALESCE(COUNT(DISTINCT f.id), 0) AS total_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'CRITICAL' THEN f.id END), 0) AS critical_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'HIGH' THEN f.id END), 0) AS high_findings,
			CASE 
				WHEN COUNT(DISTINCT r.id) = 0 THEN 100.0
				ELSE (COUNT(DISTINCT CASE WHEN r.findings_count = 0 THEN r.id END)::FLOAT / COUNT(DISTINCT r.id)::FLOAT) * 100.0
			END AS pass_rate,
			(SELECT COUNT(*) FROM tracked_repositories WHERE workspace_id = $1 AND is_active = TRUE) AS active_repos,
			(SELECT COUNT(*) FROM account_profiles WHERE workspace_id = $1) AS total_devs
		FROM pull_request_reviews r
		LEFT JOIN code_findings f ON f.review_id = r.id AND f.workspace_id = $1
		WHERE r.workspace_id = $1;
	`

	var m models.CockpitMetrics
	err := r.client.Pool.QueryRow(ctx, query, wsID).Scan(
		&m.TotalReviews,
		&m.TotalFindings,
		&m.CriticalFindings,
		&m.HighFindings,
		&m.PassRatePercentage,
		&m.ActiveRepositories,
		&m.TotalDevelopers,
	)
	if err != nil {
		return nil, fmt.Errorf("failed querying cockpit metrics: %w", err)
	}

	return &m, nil
}

// TimeoutStaleReviews identifies in-flight reviews that exceeded recovery thresholds and marks them FAILED.
func (r *Repository) TimeoutStaleReviews(ctx context.Context, olderThanMinutes int) (int64, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, nil
	}

	query := `
		UPDATE pull_request_reviews
		SET state = 'FAILED', completed_at = NOW()
		WHERE state IN ('PROCESSING', 'QUEUED', 'RECEIVED')
		  AND created_at < NOW() - ($1 || ' minutes')::interval;
	`
	tag, err := r.client.Pool.Exec(ctx, query, fmt.Sprintf("%d", olderThanMinutes))
	if err != nil {
		return 0, fmt.Errorf("failed timing out stale reviews: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneExpiredCLISessions deletes expired terminal authorization device sessions to prevent replay attacks.
func (r *Repository) PruneExpiredCLISessions(ctx context.Context) (int64, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, nil
	}

	query := `
		DELETE FROM cli_auth_sessions
		WHERE expires_at < NOW()
		   OR (status = 'consumed' AND "updatedAt" < NOW() - INTERVAL '24 hours');
	`
	tag, err := r.client.Pool.Exec(ctx, query)
	if err != nil {
		return 0, fmt.Errorf("failed pruning expired CLI sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}

// PruneInactiveLicenseSeats downgrades inactive members to VIEWER to reclaim allocated seats.
func (r *Repository) PruneInactiveLicenseSeats(ctx context.Context, inactivityDays int) (int64, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return 0, nil
	}

	query := `
		UPDATE account_profiles
		SET role = 'VIEWER', updated_at = NOW()
		WHERE UPPER(role) NOT IN ('OWNER', 'ADMIN', 'VIEWER')
		  AND COALESCE(last_active_at, updated_at) < NOW() - ($1 || ' days')::interval;
	`
	tag, err := r.client.Pool.Exec(ctx, query, fmt.Sprintf("%d", inactivityDays))
	if err != nil {
		return 0, fmt.Errorf("failed pruning inactive license seats: %w", err)
	}
	return tag.RowsAffected(), nil
}

// TouchAccountActivity updates last_active_at for an account profile within a tenant boundary.
func (r *Repository) TouchAccountActivity(ctx context.Context, wsID uuid.UUID, email string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	query := `UPDATE account_profiles SET last_active_at = NOW() WHERE workspace_id = $1 AND email = $2;`
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, wsID, email)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// Profile may not exist yet in account_profiles, upsert it
			upsertQuery := `
				INSERT INTO account_profiles (id, workspace_id, email, display_name, role, last_active_at, created_at, updated_at)
				VALUES (gen_random_uuid(), $1, $2, $2, 'MEMBER', NOW(), NOW(), NOW())
				ON CONFLICT (workspace_id, email) DO UPDATE SET last_active_at = NOW(), updated_at = NOW();
			`
			_, err = tx.Exec(ctx, upsertQuery, wsID, email)
			return err
		}
		return nil
	})
}

// GetBillingTransaction retrieves a billing transaction record by workspace and order ID.
func (r *Repository) GetBillingTransaction(ctx context.Context, wsID uuid.UUID, orderID string) (*models.BillingTransaction, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, nil
	}
	query := `
		SELECT id, workspace_id, provider, order_id, COALESCE(payment_id, ''), COALESCE(signature, ''),
		       amount, currency, plan_tier, status, COALESCE(receipt, ''), created_at, updated_at
		FROM billing_transactions
		WHERE workspace_id = $1 AND order_id = $2
		LIMIT 1;
	`
	var tx models.BillingTransaction
	err := r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		return pgTx.QueryRow(ctx, query, wsID, orderID).Scan(
			&tx.ID, &tx.WorkspaceID, &tx.Provider, &tx.OrderID, &tx.PaymentID, &tx.Signature,
			&tx.Amount, &tx.Currency, &tx.PlanTier, &tx.Status, &tx.Receipt, &tx.CreatedAt, &tx.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

// ClaimBillingUpgrade atomically claims an order for upgrade processing, preventing duplicate webhook/checkout races.
func (r *Repository) ClaimBillingUpgrade(ctx context.Context, wsID uuid.UUID, orderID string) (bool, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return true, nil
	}
	query := `
		UPDATE billing_transactions
		SET status = 'processing_upgrade', updated_at = NOW()
		WHERE workspace_id = $1 AND order_id = $2 AND status != 'captured' AND status != 'processing_upgrade';
	`
	var rowsAffected int64
	err := r.client.ExecWithTenant(ctx, wsID, func(pgTx pgx.Tx) error {
		tag, err := pgTx.Exec(ctx, query, wsID, orderID)
		if err == nil {
			rowsAffected = tag.RowsAffected()
		}
		return err
	})
	return rowsAffected > 0, err
}

// AggregateDORARollup calculates rolling aggregate metrics across all active workspace repositories.
func (r *Repository) AggregateDORARollup(ctx context.Context) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}
	query := `
		INSERT INTO warehouse_domain_events (
			id, workspace_id, aggregate_id, aggregate_type, event_type, version, payload, metadata, occurred_at, created_at
		)
		SELECT gen_random_uuid(), w.id, w.id, 'workspace', 'dora.metrics.rollup', 1,
		       json_build_object(
		           'review_count', (SELECT COUNT(*) FROM pull_request_reviews r WHERE r.workspace_id = w.id),
		           'findings_count', (SELECT COUNT(*) FROM code_findings f WHERE f.workspace_id = w.id),
		           'calculated_at', NOW()
		       ),
		       '{}'::jsonb,
		       NOW(),
		       NOW()
		FROM workspaces w
		WHERE w.status = 'ACTIVE'
		ON CONFLICT DO NOTHING;
	`
	_, err := r.client.Pool.Exec(ctx, query)
	return err
}

// CreateWorkspaceWithUser atomically provisions a workspace, owner user account, and profile in a single transaction.
func (r *Repository) CreateWorkspaceWithUser(ctx context.Context, ws *models.Workspace, email, passwordHash, role, displayName string) (*UserRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	now := time.Now().UTC()
	ws.CreatedAt = now
	ws.UpdatedAt = now
	if ws.Status == "" {
		ws.Status = models.TenantStatusActive
	}
	if role == "" {
		role = "owner"
	}
	if displayName == "" {
		displayName = email
	}

	var userRecord UserRecord
	err := r.client.ExecWithTenant(ctx, ws.ID, func(tx pgx.Tx) error {
		// 1. Insert Workspace
		queryWS := `
			INSERT INTO workspaces (id, slug, name, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6);
		`
		if _, err := tx.Exec(ctx, queryWS, ws.ID, ws.Slug, ws.Name, ws.Status, ws.CreatedAt, ws.UpdatedAt); err != nil {
			return fmt.Errorf("failed creating workspace: %w", err)
		}

		// 2. Insert User linked to Workspace
		newUserID := uuid.New()
		queryUser := `
			INSERT INTO users (uuid, email, password, role, status, organization_id, "createdAt", "updatedAt")
			VALUES ($1, $2, $3, $4::users_role_enum, 'active', $5, now(), now())
			RETURNING uuid, email, password, role, status, organization_id, "createdAt", "updatedAt";
		`
		err := tx.QueryRow(ctx, queryUser, newUserID, email, passwordHash, role, ws.ID).Scan(
			&userRecord.UUID, &userRecord.Email, &userRecord.Password, &userRecord.Role,
			&userRecord.Status, &userRecord.OrganizationID, &userRecord.CreatedAt, &userRecord.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed creating user account: %w", err)
		}

		// 3. Insert Initial Account Profile
		queryProfile := `
			INSERT INTO account_profiles (id, workspace_id, email, display_name, role, last_active_at, created_at, updated_at)
			VALUES (gen_random_uuid(), $1, $2, $3, UPPER($4), now(), now(), now())
			ON CONFLICT (workspace_id, email) DO UPDATE SET last_active_at = now(), updated_at = now();
		`
		if _, err := tx.Exec(ctx, queryProfile, ws.ID, email, displayName, role); err != nil {
			return fmt.Errorf("failed creating account profile: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &userRecord, nil
}

// SecurityMemoryRecord models semantic review findings in PostgreSQL pgvector memory.
type SecurityMemoryRecord struct {
	ID                 uuid.UUID `json:"id"`
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	FindingFingerprint string    `json:"finding_fingerprint"`
	Category           string    `json:"category"`
	RuleID             string    `json:"rule_id"`
	CodeSnippet        string    `json:"code_snippet"`
	Justification      string    `json:"justification"`
	DismissalReason    string    `json:"dismissal_reason"`
	SimilarityScore    float64   `json:"similarity_score,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// SaveSecurityMemory stores a semantic finding into pgvector security memory.
func (r *Repository) SaveSecurityMemory(ctx context.Context, wsID uuid.UUID, mem *SecurityMemoryRecord) error {
	return r.UpsertSecurityMemoryWithEmbedding(ctx, wsID, mem, nil)
}

// UpsertSecurityMemoryWithEmbedding stores or updates a security finding vector in pgvector memory.
func (r *Repository) UpsertSecurityMemoryWithEmbedding(ctx context.Context, wsID uuid.UUID, mem *SecurityMemoryRecord, embedding []float32) error {
	if r == nil || r.client == nil {
		return nil
	}
	if mem.ID == uuid.Nil {
		mem.ID = uuid.New()
	}
	mem.WorkspaceID = wsID
	mem.CreatedAt = time.Now().UTC()

	var query string
	var args []any

	if len(embedding) > 0 {
		vecStr := formatVector(embedding)
		query = `
			INSERT INTO security_memory (
				id, workspace_id, finding_fingerprint, category, rule_id,
				code_snippet, justification, dismissal_reason, embedding, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector, $10)
			ON CONFLICT (id) DO UPDATE SET
				code_snippet = EXCLUDED.code_snippet,
				justification = EXCLUDED.justification,
				dismissal_reason = EXCLUDED.dismissal_reason,
				embedding = EXCLUDED.embedding;
		`
		args = []any{
			mem.ID, mem.WorkspaceID, mem.FindingFingerprint, mem.Category, mem.RuleID,
			mem.CodeSnippet, mem.Justification, mem.DismissalReason, vecStr, mem.CreatedAt,
		}
	} else {
		query = `
			INSERT INTO security_memory (
				id, workspace_id, finding_fingerprint, category, rule_id,
				code_snippet, justification, dismissal_reason, embedding, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, array_fill(0.0::real, ARRAY[1536])::vector, $9)
			ON CONFLICT (id) DO NOTHING;
		`
		args = []any{
			mem.ID, mem.WorkspaceID, mem.FindingFingerprint, mem.Category, mem.RuleID,
			mem.CodeSnippet, mem.Justification, mem.DismissalReason, mem.CreatedAt,
		}
	}

	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	})
}

// SearchSimilarSecurityFindings executes cosine distance semantic similarity vector search in pgvector.
func (r *Repository) SearchSimilarSecurityFindings(ctx context.Context, wsID uuid.UUID, embedding []float32, category string, limit int, maxDistance float64) ([]SecurityMemoryRecord, error) {
	if r == nil || r.client == nil || len(embedding) == 0 {
		return []SecurityMemoryRecord{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if maxDistance <= 0 {
		maxDistance = 0.5
	}
	vecStr := formatVector(embedding)
	query := `
		SELECT id, workspace_id, finding_fingerprint, category, rule_id,
		       code_snippet, justification, dismissal_reason,
		       (embedding <=> $2::vector) as distance, created_at
		FROM security_memory
		WHERE workspace_id = $1 AND ($3 = '' OR category = $3)
		  AND (embedding <=> $2::vector) <= $4
		ORDER BY distance ASC
		LIMIT $5;
	`
	var results []SecurityMemoryRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, vecStr, category, maxDistance, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m SecurityMemoryRecord
			var dist float64
			if err := rows.Scan(
				&m.ID, &m.WorkspaceID, &m.FindingFingerprint, &m.Category, &m.RuleID,
				&m.CodeSnippet, &m.Justification, &m.DismissalReason, &dist, &m.CreatedAt,
			); err != nil {
				return err
			}
			m.SimilarityScore = 1.0 - dist
			results = append(results, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed semantic security memory search: %w", err)
	}
	return results, nil
}

func formatVector(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(f), 'f', 6, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}

// ListWorkspaces retrieves all active workspaces across the platform.
func (r *Repository) ListWorkspaces(ctx context.Context) ([]models.Workspace, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []models.Workspace{}, nil
	}

	query := `
		SELECT id, slug, name, status, created_at, updated_at
		FROM workspaces
		WHERE status = 'ACTIVE'
		ORDER BY created_at ASC;
	`
	rows, err := r.client.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed listing workspaces: %w", err)
	}
	defer rows.Close()

	var list []models.Workspace
	for rows.Next() {
		var w models.Workspace
		if err := rows.Scan(&w.ID, &w.Slug, &w.Name, &w.Status, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, w)
	}
	return list, nil
}

// PendingApprovalRecord represents a review eligible for automated PR approval.
type PendingApprovalRecord struct {
	ReviewID      uuid.UUID `json:"review_id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	RepositoryID  uuid.UUID `json:"repository_id"`
	PullNumber    int       `json:"pull_number"`
	Title         string    `json:"title"`
	HeadSHA       string    `json:"head_sha"`
	BaseSHA       string    `json:"base_sha"`
	FindingsCount int       `json:"findings_count"`
	CriticalCount int       `json:"critical_count"`
	HighCount     int       `json:"high_count"`
}

// GetPendingApprovalReviews queries reviews in COMPLETED state with zero critical/high blockers.
func (r *Repository) GetPendingApprovalReviews(ctx context.Context, limit int) ([]PendingApprovalRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []PendingApprovalRecord{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		SELECT 
			r.id, r.workspace_id, r.repository_id, r.pull_number, r.title, r.head_sha, r.base_sha, r.findings_count,
			COALESCE(SUM(CASE WHEN UPPER(f.severity) = 'CRITICAL' THEN 1 ELSE 0 END), 0) AS critical_count,
			COALESCE(SUM(CASE WHEN UPPER(f.severity) = 'HIGH' THEN 1 ELSE 0 END), 0) AS high_count
		FROM pull_request_reviews r
		LEFT JOIN code_findings f ON f.review_id = r.id AND f.workspace_id = r.workspace_id
		WHERE r.state = 'COMPLETED'
		  AND r.created_at >= NOW() - INTERVAL '7 days'
		GROUP BY r.id, r.workspace_id, r.repository_id, r.pull_number, r.title, r.head_sha, r.base_sha, r.findings_count
		HAVING COALESCE(SUM(CASE WHEN UPPER(f.severity) IN ('CRITICAL', 'HIGH') THEN 1 ELSE 0 END), 0) = 0
		ORDER BY r.created_at DESC
		LIMIT $1;
	`

	rows, err := r.client.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying pending approval reviews: %w", err)
	}
	defer rows.Close()

	var results []PendingApprovalRecord
	for rows.Next() {
		var p PendingApprovalRecord
		if err := rows.Scan(
			&p.ReviewID, &p.WorkspaceID, &p.RepositoryID, &p.PullNumber,
			&p.Title, &p.HeadSHA, &p.BaseSHA, &p.FindingsCount,
			&p.CriticalCount, &p.HighCount,
		); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, nil
}

// RecordReviewApproval logs an automated review approval event in the outbox and audit log.
func (r *Repository) RecordReviewApproval(ctx context.Context, wsID, reviewID uuid.UUID, pullNumber int, reason string) error {
	if r == nil || r.client == nil {
		return nil
	}

	payload, _ := json.Marshal(map[string]any{
		"review_id":   reviewID.String(),
		"workspace_id": wsID.String(),
		"pull_number": pullNumber,
		"reason":      reason,
		"approved_at": time.Now().UTC().Format(time.RFC3339),
	})

	outbox := &models.OutboxRecord{
		ID:          uuid.New(),
		WorkspaceID: wsID,
		EventType:   "pull_request.approved",
		Payload:     payload,
		Status:      models.OutboxPending,
		CreatedAt:   time.Now().UTC(),
	}

	return r.InsertOutboxEvent(ctx, outbox)
}

// SyncFindingFeedbackSentiment aggregates feedback sentiment to tune rule confidence weights.
func (r *Repository) SyncFindingFeedbackSentiment(ctx context.Context, wsID uuid.UUID) (int64, error) {
	if r == nil || r.client == nil {
		return 0, nil
	}

	query := `
		INSERT INTO warehouse_domain_events (
			id, workspace_id, aggregate_id, aggregate_type, event_type, version, payload, metadata, occurred_at, created_at
		)
		SELECT 
			gen_random_uuid(), f.workspace_id, f.finding_id, 'finding_feedback', 'feedback.sentiment.synced', 1,
			json_build_object(
				'sentiment', f.sentiment,
				'comment', f.comment,
				'synced_at', NOW()
			),
			'{}'::jsonb,
			NOW(),
			NOW()
		FROM finding_feedback f
		WHERE f.workspace_id = $1 AND f.created_at >= NOW() - INTERVAL '24 hours'
		ON CONFLICT DO NOTHING;
	`
	var affected int64
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, query, wsID)
		if err == nil {
			affected = tag.RowsAffected()
		}
		return err
	})
	return affected, err
}

// OrphanedSessionRecord models an unclosed CLI device or trace session.
type OrphanedSessionRecord struct {
	ID        uuid.UUID `json:"id"`
	SessionID string    `json:"session_id"`
	UserCode  string    `json:"user_code"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FindOrphanedCLISessions retrieves CLI sessions that have not received heartbeats within threshold.
func (r *Repository) FindOrphanedCLISessions(ctx context.Context, inactivityThresholdMinutes, limit int) ([]OrphanedSessionRecord, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []OrphanedSessionRecord{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		SELECT id, session_id, user_code, status, created_at, "updatedAt"
		FROM cli_auth_sessions
		WHERE status IN ('pending', 'authorized')
		  AND "updatedAt" < NOW() - ($1 || ' minutes')::interval
		ORDER BY "updatedAt" ASC
		LIMIT $2;
	`
	rows, err := r.client.Pool.Query(ctx, query, fmt.Sprintf("%d", inactivityThresholdMinutes), limit)
	if err != nil {
		return nil, fmt.Errorf("failed querying orphaned sessions: %w", err)
	}
	defer rows.Close()

	var sessions []OrphanedSessionRecord
	for rows.Next() {
		var s OrphanedSessionRecord
		if err := rows.Scan(&s.ID, &s.SessionID, &s.UserCode, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// MarkCLISessionClassified updates the status of an orphaned session to expired/classified.
func (r *Repository) MarkCLISessionClassified(ctx context.Context, sessionID uuid.UUID, classification string) error {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil
	}

	query := `
		UPDATE cli_auth_sessions
		SET status = 'expired', "updatedAt" = NOW()
		WHERE id = $1;
	`
	_, err := r.client.Pool.Exec(ctx, query, sessionID)
	return err
}

// SpendLimitEvaluation holds monthly usage and configured spend limit for a workspace.
type SpendLimitEvaluation struct {
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	OrganizationName   string    `json:"organization_name"`
	MonthlySpendLimit  float64   `json:"monthly_spend_limit"`
	CurrentSpendUSD    float64   `json:"current_spend_usd"`
	UsagePercentage    float64   `json:"usage_percentage"`
	TotalTokensUsed    int64     `json:"total_tokens_used"`
	OwnerEmail         string    `json:"owner_email"`
}

// GetWorkspacesSpendEvaluation evaluates current month spend against limits across all workspaces.
func (r *Repository) GetWorkspacesSpendEvaluation(ctx context.Context) ([]SpendLimitEvaluation, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []SpendLimitEvaluation{}, nil
	}

	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)

	query := `
		SELECT 
			w.id AS workspace_id,
			w.name AS organization_name,
			COALESCE(sl.monthly_spend_limit_usd, 50.0) AS spend_limit,
			COALESCE(SUM(t.cost_usd), 0.0) AS current_spend,
			COALESCE(SUM(t.prompt_tokens + t.completion_tokens), 0) AS total_tokens,
			COALESCE((
				SELECT email FROM account_profiles 
				WHERE workspace_id = w.id 
				ORDER BY CASE WHEN UPPER(role) = 'OWNER' THEN 1 WHEN UPPER(role) = 'ADMIN' THEN 2 ELSE 3 END 
				LIMIT 1
			), '') AS owner_email
		FROM workspaces w
		LEFT JOIN workspace_spend_limits sl ON sl.workspace_id = w.id
		LEFT JOIN token_usage_records t ON t.workspace_id = w.id AND t.created_at >= $1
		WHERE w.status = 'ACTIVE'
		GROUP BY w.id, w.name, sl.monthly_spend_limit_usd;
	`

	rows, err := r.client.Pool.Query(ctx, query, startOfMonth)
	if err != nil {
		return nil, fmt.Errorf("failed querying workspaces spend evaluation: %w", err)
	}
	defer rows.Close()

	var evaluations []SpendLimitEvaluation
	for rows.Next() {
		var e SpendLimitEvaluation
		if err := rows.Scan(
			&e.WorkspaceID, &e.OrganizationName, &e.MonthlySpendLimit,
			&e.CurrentSpendUSD, &e.TotalTokensUsed, &e.OwnerEmail,
		); err != nil {
			return nil, err
		}
		if e.MonthlySpendLimit > 0 {
			e.UsagePercentage = (e.CurrentSpendUSD / e.MonthlySpendLimit) * 100.0
		} else {
			e.UsagePercentage = 0
		}
		evaluations = append(evaluations, e)
	}
	return evaluations, nil
}

// RepoReportData contains aggregated performance and quality metrics for a repository digest.
type RepoReportData struct {
	WorkspaceID          uuid.UUID `json:"workspace_id"`
	RepositoryID         uuid.UUID `json:"repository_id"`
	NamespacePath        string    `json:"namespace_path"`
	TotalReviews         int       `json:"total_reviews"`
	CleanReviewsCount    int       `json:"clean_reviews_count"`
	PassRate             float64   `json:"pass_rate"`
	TotalFindings        int       `json:"total_findings"`
	CriticalFindings     int       `json:"critical_findings"`
	HighFindings         int       `json:"high_findings"`
	ActiveContributors   int       `json:"active_contributors"`
}

// GetRepositoryReportsData compiles performance metrics for active repositories over a time window.
func (r *Repository) GetRepositoryReportsData(ctx context.Context, since time.Time) ([]RepoReportData, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return []RepoReportData{}, nil
	}

	query := `
		SELECT 
			repo.workspace_id,
			repo.id AS repository_id,
			repo.namespace_path,
			COUNT(DISTINCT rev.id) AS total_reviews,
			COUNT(DISTINCT CASE WHEN rev.findings_count = 0 THEN rev.id END) AS clean_reviews_count,
			COALESCE(COUNT(DISTINCT f.id), 0) AS total_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'CRITICAL' THEN f.id END), 0) AS critical_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'HIGH' THEN f.id END), 0) AS high_findings,
			COUNT(DISTINCT rev.author_username) AS active_contributors
		FROM tracked_repositories repo
		LEFT JOIN pull_request_reviews rev ON rev.repository_id = repo.id AND rev.created_at >= $1
		LEFT JOIN code_findings f ON f.review_id = rev.id
		WHERE repo.is_active = TRUE
		GROUP BY repo.workspace_id, repo.id, repo.namespace_path
		HAVING COUNT(DISTINCT rev.id) > 0;
	`

	rows, err := r.client.Pool.Query(ctx, query, since)
	if err != nil {
		return nil, fmt.Errorf("failed compiling repository reports data: %w", err)
	}
	defer rows.Close()

	var reports []RepoReportData
	for rows.Next() {
		var rd RepoReportData
		if err := rows.Scan(
			&rd.WorkspaceID, &rd.RepositoryID, &rd.NamespacePath,
			&rd.TotalReviews, &rd.CleanReviewsCount,
			&rd.TotalFindings, &rd.CriticalFindings, &rd.HighFindings,
			&rd.ActiveContributors,
		); err != nil {
			return nil, err
		}
		if rd.TotalReviews > 0 {
			rd.PassRate = (float64(rd.CleanReviewsCount) / float64(rd.TotalReviews)) * 100.0
		} else {
			rd.PassRate = 100.0
		}
		reports = append(reports, rd)
	}
	return reports, nil
}

// GetSecurityMemoryByFingerprint checks if a finding has been recorded as a suppressed false positive.
func (r *Repository) GetSecurityMemoryByFingerprint(ctx context.Context, wsID uuid.UUID, fingerprint string) (*SecurityMemoryRecord, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	var mem SecurityMemoryRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		query := `
			SELECT id, workspace_id, finding_fingerprint, category, rule_id,
			       code_snippet, justification, dismissal_reason, created_at
			FROM security_memory
			WHERE workspace_id = $1 AND finding_fingerprint = $2
			LIMIT 1
		`
		return tx.QueryRow(ctx, query, wsID, fingerprint).Scan(
			&mem.ID, &mem.WorkspaceID, &mem.FindingFingerprint, &mem.Category, &mem.RuleID,
			&mem.CodeSnippet, &mem.Justification, &mem.DismissalReason, &mem.CreatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &mem, nil
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

// LiveQuotaStatus represents multi-tenant token consumption and quota thresholds.
type LiveQuotaStatus struct {
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	Tier               string    `json:"tier"`
	BYOKEnabled        bool      `json:"byok_enabled"`
	MonthlyTokenLimit  int64     `json:"monthly_token_limit"`
	TokensUsedThisMonth int64    `json:"tokens_used_this_month"`
	TokensRemaining    int64     `json:"tokens_remaining"`
	PercentUsed        float64   `json:"percent_used"`
	EstimatedCostUSD   float64   `json:"estimated_cost_usd"`
	BurstLimitPerMin   int64     `json:"burst_limit_per_min"`
	BillingPeriodStart time.Time `json:"billing_period_start"`
	BillingPeriodEnd   time.Time `json:"billing_period_end"`
}

// GetLiveTokenQuota evaluates real-time token usage against active plan allocation and spend caps.
func (r *Repository) GetLiveTokenQuota(ctx context.Context, wsID uuid.UUID) (*LiveQuotaStatus, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	if r == nil || r.client == nil {
		return &LiveQuotaStatus{
			WorkspaceID:        wsID,
			Tier:               "Free",
			BYOKEnabled:        false,
			MonthlyTokenLimit:  500_000,
			TokensUsedThisMonth: 0,
			TokensRemaining:    500_000,
			PercentUsed:        0,
			EstimatedCostUSD:   0,
			BurstLimitPerMin:   50_000,
			BillingPeriodStart: startOfMonth,
			BillingPeriodEnd:   endOfMonth,
		}, nil
	}

	promptTokens, compTokens, costUSD, err := r.GetWorkspaceUsage(ctx, wsID, startOfMonth)
	if err != nil {
		return nil, err
	}
	totalUsed := promptTokens + compTokens

	// Query allocation and BYOK flag
	var tier string
	var byokEnabled bool
	allocQuery := `SELECT tier, byok_enabled FROM billing_seat_allocations WHERE workspace_id = $1 LIMIT 1;`
	_ = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, allocQuery, wsID).Scan(&tier, &byokEnabled)
	})

	if tier == "" {
		tier = "Free"
	}

	var monthlyLimit int64
	var burstLimit int64

	switch strings.ToUpper(tier) {
	case "PRO":
		monthlyLimit = 10_000_000
		burstLimit = 500_000
	case "ENTERPRISE":
		monthlyLimit = 50_000_000
		burstLimit = 2_000_000
	default:
		monthlyLimit = 500_000
		burstLimit = 50_000
	}

	remaining := monthlyLimit - totalUsed
	if remaining < 0 {
		remaining = 0
	}

	pctUsed := 0.0
	if monthlyLimit > 0 {
		pctUsed = (float64(totalUsed) / float64(monthlyLimit)) * 100.0
	}

	return &LiveQuotaStatus{
		WorkspaceID:        wsID,
		Tier:               tier,
		BYOKEnabled:        byokEnabled,
		MonthlyTokenLimit:  monthlyLimit,
		TokensUsedThisMonth: totalUsed,
		TokensRemaining:    remaining,
		PercentUsed:        pctUsed,
		EstimatedCostUSD:   costUSD,
		BurstLimitPerMin:   burstLimit,
		BillingPeriodStart: startOfMonth,
		BillingPeriodEnd:   endOfMonth,
	}, nil
}

// DailyUsageSummary holds daily token metrics for usage graphs.
type DailyUsageSummary struct {
	Date             string  `json:"date"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// GetWorkspaceDailyUsageHistory aggregates daily token metrics for a lookback window.
func (r *Repository) GetWorkspaceDailyUsageHistory(ctx context.Context, wsID uuid.UUID, days int) ([]DailyUsageSummary, error) {
	if r == nil || r.client == nil {
		return []DailyUsageSummary{}, nil
	}
	if days <= 0 || days > 90 {
		days = 30
	}

	since := time.Now().UTC().AddDate(0, 0, -days)
	query := `
		SELECT 
			TO_CHAR(created_at, 'YYYY-MM-DD') AS day,
			COALESCE(SUM(prompt_tokens), 0) AS p_tokens,
			COALESCE(SUM(completion_tokens), 0) AS c_tokens,
			COALESCE(SUM(cost_usd), 0.0) AS daily_cost
		FROM token_usage_records
		WHERE workspace_id = $1 AND created_at >= $2
		GROUP BY TO_CHAR(created_at, 'YYYY-MM-DD')
		ORDER BY day ASC;
	`

	var summaries []DailyUsageSummary
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, since)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s DailyUsageSummary
			if err := rows.Scan(&s.Date, &s.PromptTokens, &s.CompletionTokens, &s.CostUSD); err != nil {
				return err
			}
			s.TotalTokens = s.PromptTokens + s.CompletionTokens
			summaries = append(summaries, s)
		}
		return rows.Err()
	})
	return summaries, err
}

// InsertReviewAttestation stores a cryptographically signed DSSE attestation envelope.
func (r *Repository) InsertReviewAttestation(ctx context.Context, rec intoto.AttestationRecord) error {
	if r == nil || r.client == nil {
		return nil
	}

	envBytes, err := json.Marshal(rec.Envelope)
	if err != nil {
		return fmt.Errorf("failed serializing attestation envelope: %w", err)
	}

	if rec.ID == uuid.Nil {
		rec.ID = uuid.New()
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}

	query := `
		INSERT INTO review_attestations (
			id, workspace_id, review_id, predicate_type, decision, key_id, envelope_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (workspace_id, review_id, predicate_type)
		DO UPDATE SET
			decision = EXCLUDED.decision,
			key_id = EXCLUDED.key_id,
			envelope_json = EXCLUDED.envelope_json,
			created_at = EXCLUDED.created_at;
	`

	return r.client.ExecWithTenant(ctx, rec.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			rec.ID, rec.WorkspaceID, rec.ReviewID, rec.PredicateType, rec.Decision, rec.KeyID, envBytes, rec.CreatedAt,
		)
		return err
	})
}

// GetReviewAttestations retrieves all cryptographic attestations for a given review.
func (r *Repository) GetReviewAttestations(ctx context.Context, wsID, reviewID uuid.UUID) ([]intoto.AttestationRecord, error) {
	if r == nil || r.client == nil {
		return []intoto.AttestationRecord{}, nil
	}

	query := `
		SELECT id, workspace_id, review_id, predicate_type, decision, key_id, envelope_json, created_at
		FROM review_attestations
		WHERE workspace_id = $1 AND review_id = $2
		ORDER BY created_at DESC;
	`

	var records []intoto.AttestationRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, reviewID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rec intoto.AttestationRecord
			var envBytes []byte
			if err := rows.Scan(
				&rec.ID, &rec.WorkspaceID, &rec.ReviewID, &rec.PredicateType,
				&rec.Decision, &rec.KeyID, &envBytes, &rec.CreatedAt,
			); err != nil {
				return err
			}
			if len(envBytes) > 0 {
				_ = json.Unmarshal(envBytes, &rec.Envelope)
			}
			records = append(records, rec)
		}
		return rows.Err()
	})

	return records, err
}

// GetLatestReviewAttestation returns the most recent in-toto / SLSA attestation for a review.
func (r *Repository) GetLatestReviewAttestation(ctx context.Context, wsID, reviewID uuid.UUID) (*intoto.AttestationRecord, error) {
	records, err := r.GetReviewAttestations(ctx, wsID, reviewID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return &records[0], nil
}



