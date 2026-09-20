// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Database Access Layer
// File: rules_repository.go
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/automation"
	"github.com/scandrix/backend/pkg/models"
)

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


// CreateReviewRule registers a new custom Drixy review rule in PostgreSQL.
func (r *Repository) CreateReviewRule(ctx context.Context, rule *models.ReviewRule) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		INSERT INTO review_rules (id, workspace_id, repository_id, name, description, severity, rule_type, rule_content, is_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	now := time.Now().UTC()
	if rule.ID == uuid.Nil {
		rule.ID = uuid.New()
	}
	rule.CreatedAt = now
	rule.UpdatedAt = now
	if rule.Severity == "" {
		rule.Severity = models.SeverityMedium
	}
	if rule.RuleType == "" {
		rule.RuleType = "REGEX"
	}

	return r.client.ExecWithTenant(ctx, rule.WorkspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query,
			rule.ID, rule.WorkspaceID, rule.RepositoryID, rule.Name, rule.Description,
			string(rule.Severity), rule.RuleType, rule.RuleContent, rule.IsEnabled,
			rule.CreatedAt, rule.UpdatedAt,
		)
		return err
	})
}


// ListReviewRules retrieves all custom review rules for a workspace.
func (r *Repository) ListReviewRules(ctx context.Context, workspaceID uuid.UUID) ([]models.ReviewRule, error) {
	if r == nil || r.client == nil {
		return []models.ReviewRule{}, nil
	}

	query := `
		SELECT id, workspace_id, repository_id, name, description, severity, rule_type, rule_content, is_enabled, created_at, updated_at
		FROM review_rules
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`
	var list []models.ReviewRule
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var rule models.ReviewRule
			var sev string
			if err := rows.Scan(
				&rule.ID, &rule.WorkspaceID, &rule.RepositoryID, &rule.Name, &rule.Description,
				&sev, &rule.RuleType, &rule.RuleContent, &rule.IsEnabled,
				&rule.CreatedAt, &rule.UpdatedAt,
			); err != nil {
				return err
			}
			rule.Severity = models.FindingSeverity(sev)
			list = append(list, rule)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}


// DeleteReviewRule removes a custom review rule from PostgreSQL.
func (r *Repository) DeleteReviewRule(ctx context.Context, workspaceID, ruleID uuid.UUID) error {
	if r == nil || r.client == nil {
		return nil
	}

	query := `
		DELETE FROM review_rules
		WHERE workspace_id = $1 AND id = $2
	`
	return r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, workspaceID, ruleID)
		return err
	})
}


// SyncRulesWithPlanLimit enforces plan rule quotas by disabling oldest excess active rules.
// If maxAllowedRules <= 0 (e.g. Enterprise tier), all active rules remain untouched.
// Returns the count of rules that were disabled.
func (r *Repository) SyncRulesWithPlanLimit(ctx context.Context, workspaceID uuid.UUID, maxAllowedRules int) (int, error) {
	if r == nil || r.client == nil || maxAllowedRules <= 0 {
		return 0, nil
	}

	query := `
		SELECT id
		FROM review_rules
		WHERE workspace_id = $1 AND is_enabled = true
		ORDER BY created_at DESC
	`

	var disabledCount int
	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID)
		if err != nil {
			return err
		}
		defer rows.Close()

		var activeIDs []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			activeIDs = append(activeIDs, id)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if len(activeIDs) <= maxAllowedRules {
			return nil
		}

		excessIDs := activeIDs[maxAllowedRules:]
		disabledCount = len(excessIDs)

		updateQuery := `
			UPDATE review_rules
			SET is_enabled = false, updated_at = NOW()
			WHERE workspace_id = $1 AND id = ANY($2)
		`
		_, err = tx.Exec(ctx, updateQuery, workspaceID, excessIDs)
		return err
	})

	return disabledCount, err
}

