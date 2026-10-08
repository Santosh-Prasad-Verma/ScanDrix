package database

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/pkg/models"
)

func (r *Repository) GetRepositoryReviewSettings(ctx context.Context, wsID, repoID uuid.UUID) (models.RepositoryReviewSettings, error) {
	var raw, configRaw []byte
	var active bool
	if r == nil || r.client == nil {
		return models.RepositoryReviewSettings{}, errors.New("database unavailable")
	}
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT is_active AND is_tracked, review_settings, code_review_config FROM tracked_repositories WHERE workspace_id=$1 AND id=$2`, wsID, repoID).Scan(&active, &raw, &configRaw)
	})
	if err != nil {
		return models.RepositoryReviewSettings{}, err
	}
	return models.DecodeRepositoryReviewSettings(raw, active, configRaw)
}

// JSONB merge in a single UPDATE preserves independently edited fields and keeps
// active monitoring and review policy in the same tenant-scoped transaction.
func (r *Repository) PatchRepositoryReviewSettings(ctx context.Context, wsID, repoID uuid.UUID, patch models.RepositoryReviewSettingsPatch) (models.RepositoryReviewSettings, error) {
	if r == nil || r.client == nil {
		return models.RepositoryReviewSettings{}, errors.New("database unavailable")
	}
	if err := patch.Validate(); err != nil {
		return models.RepositoryReviewSettings{}, err
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return models.RepositoryReviewSettings{}, err
	}

	// review_settings keeps its shallow-merge behaviour so independently edited
	// CLI fields survive. code_review_config is a single document, so it is
	// replaced wholesale rather than merged: merging would leave a key the user
	// just removed behind in the stored row.
	var configArg any
	if patch.CodeReviewConfig != nil {
		encoded, err := json.Marshal(*patch.CodeReviewConfig)
		if err != nil {
			return models.RepositoryReviewSettings{}, err
		}
		configArg = string(encoded)
	}

	var saved, savedConfig []byte
	var active bool
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE tracked_repositories
			SET is_active = COALESCE($3::boolean, is_active),
			    review_settings = review_settings || $4::jsonb,
			    code_review_config = COALESCE($5::jsonb, code_review_config),
			    updated_at = now()
			WHERE workspace_id=$1 AND id=$2 AND is_tracked
			RETURNING is_active, review_settings, code_review_config`,
			wsID, repoID, patch.Active, string(raw), configArg,
		).Scan(&active, &saved, &savedConfig)
	})
	if err != nil {
		return models.RepositoryReviewSettings{}, err
	}
	return models.DecodeRepositoryReviewSettings(saved, active, savedConfig)
}

func (r *Repository) UntrackRepository(ctx context.Context, wsID, repoID uuid.UUID) error {
	if r == nil || r.client == nil {
		return errors.New("database unavailable")
	}
	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE tracked_repositories SET is_active=FALSE,is_tracked=FALSE,updated_at=now() WHERE workspace_id=$1 AND id=$2 AND is_tracked`, wsID, repoID)
		if err == nil && result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
}
