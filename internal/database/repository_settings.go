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
	var raw []byte
	var active bool
	if r == nil || r.client == nil {
		return models.RepositoryReviewSettings{}, errors.New("database unavailable")
	}
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT is_active AND is_tracked, review_settings FROM tracked_repositories WHERE workspace_id=$1 AND id=$2`, wsID, repoID).Scan(&active, &raw)
	})
	if err != nil {
		return models.RepositoryReviewSettings{}, err
	}
	return models.DecodeRepositoryReviewSettings(raw, active)
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
	var saved []byte
	var active bool
	err = r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `UPDATE tracked_repositories SET is_active=COALESCE($3::boolean,is_active), review_settings=review_settings || $4::jsonb, updated_at=now() WHERE workspace_id=$1 AND id=$2 AND is_tracked RETURNING is_active,review_settings`, wsID, repoID, patch.Active, string(raw)).Scan(&active, &saved)
	})
	if err != nil {
		return models.RepositoryReviewSettings{}, err
	}
	return models.DecodeRepositoryReviewSettings(saved, active)
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
