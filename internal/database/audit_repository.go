// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: audit_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

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

// GetTeamByID retrieves a team by its unique ID.
func (r *Repository) GetTeamByID(ctx context.Context, wsID, teamID uuid.UUID) (*models.Team, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database unavailable")
	}

	query := `
		SELECT id, workspace_id, name, description, created_at, updated_at
		FROM teams
		WHERE id = $1 AND workspace_id = $2
		LIMIT 1;
	`
	// Tenant-scoped: teams has RLS. The wsID is also matched in the predicate so
	// a caller cannot read another tenant's team by guessing an id.
	var t models.Team
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, teamID, wsID).Scan(
			&t.ID, &t.WorkspaceID, &t.Name, &t.Description, &t.CreatedAt, &t.UpdatedAt,
		)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// AddTeamMember adds a user to a team.
func (r *Repository) AddTeamMember(ctx context.Context, wsID, teamID, userID uuid.UUID, email, role string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `
		INSERT INTO team_members (id, team_id, user_id, email, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (team_id, user_id) DO UPDATE
		SET role = EXCLUDED.role, email = EXCLUDED.email;
	`
	// Tenant-scoped: team_members has RLS.
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, query, uuid.New(), teamID, userID, email, role, time.Now().UTC())
		return execErr
	})
}

// ListTeamMembers lists all members of a team.
func (r *Repository) ListTeamMembers(ctx context.Context, wsID, teamID uuid.UUID) ([]models.TeamMember, error) {
	if r == nil || r.client == nil {
		return []models.TeamMember{}, nil
	}

	query := `
		SELECT id, team_id, user_id, email, role, created_at
		FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.team_id = $1 AND t.workspace_id = $2
		ORDER BY tm.created_at ASC;
	`
	// Tenant-scoped: team_members has RLS, and the join keeps a caller from
	// reading the roster of a team in another workspace.
	var members []models.TeamMember
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, teamID, wsID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m models.TeamMember
			if err := rows.Scan(&m.ID, &m.TeamID, &m.UserID, &m.Email, &m.Role, &m.CreatedAt); err != nil {
				return err
			}
			members = append(members, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return members, nil
}

// RemoveTeamMember removes a user from a team.
func (r *Repository) RemoveTeamMember(ctx context.Context, wsID, teamID, userID uuid.UUID) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("database unavailable")
	}

	query := `DELETE FROM team_members WHERE team_id = $1 AND user_id = $2;`
	// Tenant-scoped: team_members has RLS.
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, query, teamID, userID)
		return execErr
	})
	return err
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

// GetCockpitMetrics aggregates real review, finding, repository, and developer stats for a workspace.
//
// In compliance with AGENTS.md Rule 2.7 an absent metric is reported as absent.
// A workspace with no reviews has NO pass rate; it previously reported 100.0,
// which told a new customer their security posture was flawless
// (AUDIT_REMEDIATION.md F-07).
func (r *Repository) GetCockpitMetrics(ctx context.Context, wsID uuid.UUID) (*models.CockpitMetrics, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		// No data source at all. Say so rather than inventing a score.
		return &models.CockpitMetrics{
			Unavailable: []string{"pass_rate_percentage:no_data_source"},
		}, nil
	}

	query := `
		SELECT
			COUNT(DISTINCT r.id) AS total_reviews,
			COALESCE(COUNT(DISTINCT f.id), 0) AS total_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'CRITICAL' THEN f.id END), 0) AS critical_findings,
			COALESCE(COUNT(DISTINCT CASE WHEN UPPER(f.severity) = 'HIGH' THEN f.id END), 0) AS high_findings,
			-- NULL, not 100.0: a pass rate is undefined without reviews.
			CASE
				WHEN COUNT(DISTINCT r.id) = 0 THEN NULL
				ELSE (COUNT(DISTINCT CASE WHEN r.findings_count = 0 THEN r.id END)::FLOAT / COUNT(DISTINCT r.id)::FLOAT) * 100.0
			END AS pass_rate,
			(SELECT COUNT(*) FROM tracked_repositories WHERE workspace_id = $1 AND is_active = TRUE) AS active_repos,
			(SELECT COUNT(*) FROM account_profiles WHERE workspace_id = $1) AS total_devs
		FROM pull_request_reviews r
		LEFT JOIN code_findings f ON f.review_id = r.id AND f.workspace_id = $1
		WHERE r.workspace_id = $1;
	`

	m := &models.CockpitMetrics{}
	// Tenant-scoped: this reads RLS-protected review/finding tables.
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, wsID).Scan(
			&m.TotalReviews,
			&m.TotalFindings,
			&m.CriticalFindings,
			&m.HighFindings,
			&m.PassRatePercentage,
			&m.ActiveRepositories,
			&m.TotalDevelopers,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("failed querying cockpit metrics: %w", err)
	}

	// A NULL pass rate means the denominator was zero: report the reason so a
	// consumer can distinguish "nothing reviewed yet" from "everything passed".
	if m.PassRatePercentage == nil {
		m.Unavailable = append(m.Unavailable, "pass_rate_percentage:no_data_source")
	}

	return m, nil
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
	// Cross-tenant by design: this rolls up metrics for every active workspace,
	// so it runs as a system worker rather than a single tenant.
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, query)
		return execErr
	})
	return err
}

// RepoReportData contains aggregated performance and quality metrics for a repository digest.
type RepoReportData struct {
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	RepositoryID       uuid.UUID `json:"repository_id"`
	NamespacePath      string    `json:"namespace_path"`
	TotalReviews       int       `json:"total_reviews"`
	CleanReviewsCount  int       `json:"clean_reviews_count"`
	PassRate           float64   `json:"pass_rate"`
	TotalFindings      int       `json:"total_findings"`
	CriticalFindings   int       `json:"critical_findings"`
	HighFindings       int       `json:"high_findings"`
	ActiveContributors int       `json:"active_contributors"`
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

	// Cross-tenant by design: a report over every repository since a date.
	// Rows are consumed inside the transaction: returning pgx.Rows from the
	// closure and iterating after commit fails with "conn busy".
	var reports []RepoReportData
	err := r.client.ExecAsSystem(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, since)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rd RepoReportData
			if err := rows.Scan(
				&rd.WorkspaceID, &rd.RepositoryID, &rd.NamespacePath,
				&rd.TotalReviews, &rd.CleanReviewsCount,
				&rd.TotalFindings, &rd.CriticalFindings, &rd.HighFindings,
				&rd.ActiveContributors,
			); err != nil {
				return err
			}
			if rd.TotalReviews > 0 {
				rd.PassRate = (float64(rd.CleanReviewsCount) / float64(rd.TotalReviews)) * 100.0
			} else {
				rd.PassRate = 100.0
			}
			reports = append(reports, rd)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed compiling repository reports data: %w", err)
	}
	return reports, nil
}
