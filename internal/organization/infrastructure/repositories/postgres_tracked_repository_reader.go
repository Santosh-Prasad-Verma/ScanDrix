// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/pkg/models"
)

// PostgresTrackedRepositoryReader implements TrackedRepositoryReader using live Postgres storage.
type PostgresTrackedRepositoryReader struct {
	pool *pgxpool.Pool
}

// NewPostgresTrackedRepositoryReader instantiates a PostgresTrackedRepositoryReader.
func NewPostgresTrackedRepositoryReader(pool *pgxpool.Pool) *PostgresTrackedRepositoryReader {
	return &PostgresTrackedRepositoryReader{pool: pool}
}

// ListTrackedRepositories queries active tracked repositories for a workspace.
func (r *PostgresTrackedRepositoryReader) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	if r.pool == nil {
		return nil, nil
	}

	query := `
		SELECT id, workspace_id, provider, external_id, namespace_path, default_branch, is_active, created_at, updated_at
		FROM tracked_repositories
		WHERE workspace_id = $1 AND is_active = true
		ORDER BY updated_at DESC
	`
	rows, err := r.pool.Query(ctx, query, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed querying tracked_repositories: %w", err)
	}
	defer rows.Close()

	var repos []models.TrackedRepository
	for rows.Next() {
		var repo models.TrackedRepository
		if err := rows.Scan(
			&repo.ID, &repo.WorkspaceID, &repo.Provider, &repo.ExternalID,
			&repo.NamespacePath, &repo.DefaultBranch, &repo.IsActive,
			&repo.CreatedAt, &repo.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed scanning tracked repository: %w", err)
		}
		repos = append(repos, repo)
	}

	return repos, nil
}
