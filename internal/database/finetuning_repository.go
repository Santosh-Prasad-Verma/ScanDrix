package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/finetuning"
)

// GetFineTuningSamples retrieves human-reviewed findings and feedback pairs for fine-tuning dataset generation.
func (r *Repository) GetFineTuningSamples(
	ctx context.Context,
	workspaceID uuid.UUID,
	limit int,
) ([]finetuning.ReviewSuggestionTrainingSample, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	if limit <= 0 || limit > 5000 {
		limit = 1000
	}

	query := `
		SELECT 
			cf.id,
			cf.workspace_id,
			cf.file_path,
			COALESCE(cf.suggested_diff, ''),
			cf.title || E'\n\n' || cf.description || E'\n\nRemediation:\n' || COALESCE(cf.remediation, ''),
			COALESCE(cf.status, 'OPEN'),
			COALESCE(cf.dismissal_reason, ''),
			cf.created_at
		FROM code_findings cf
		WHERE cf.workspace_id = $1
		  AND (cf.suggested_diff IS NOT NULL AND cf.suggested_diff != '')
		ORDER BY cf.created_at DESC
		LIMIT $2
	`

	var samples []finetuning.ReviewSuggestionTrainingSample

	err := r.client.ExecWithTenant(ctx, workspaceID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, workspaceID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var (
				id              uuid.UUID
				wsID            uuid.UUID
				filePath        string
				diff            string
				suggestion      string
				status          string
				dismissalReason string
				createdAt       time.Time
			)

			if err := rows.Scan(&id, &wsID, &filePath, &diff, &suggestion, &status, &dismissalReason, &createdAt); err != nil {
				return err
			}

			fbStatus := finetuning.FeedbackAccepted
			if status == "DISMISSED" {
				fbStatus = finetuning.FeedbackRejected
			} else if status == "OPEN" {
				fbStatus = finetuning.FeedbackIgnored
			}

			samples = append(samples, finetuning.ReviewSuggestionTrainingSample{
				ID:                 id,
				WorkspaceID:        wsID,
				SystemPrompt:       "You are ScanDrix AI, an elite autonomous code reviewer specializing in AST security analysis, vulnerability remediation, and production resilience.",
				InputCodeDiff:      fmt.Sprintf("// Target File: %s\n%s", filePath, diff),
				ExpectedSuggestion: suggestion,
				Status:             fbStatus,
				CapturedAt:         createdAt,
			})
		}
		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("failed querying fine-tuning samples: %w", err)
	}

	return samples, nil
}
