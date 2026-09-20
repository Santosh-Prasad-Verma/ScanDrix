// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	clikey "github.com/scandrix/backend/internal/organization/domain/teamclikey"
)

// PostgresTeamCliKeyRepository implements ITeamCliKeyRepository.
type PostgresTeamCliKeyRepository struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[uuid.UUID]*clikey.TeamCliKeyEntity
}

// NewPostgresTeamCliKeyRepository creates a new repository.
func NewPostgresTeamCliKeyRepository(pool *pgxpool.Pool) *PostgresTeamCliKeyRepository {
	return &PostgresTeamCliKeyRepository{
		pool:   pool,
		memory: make(map[uuid.UUID]*clikey.TeamCliKeyEntity),
	}
}

// Create inserts a new CLI key.
func (r *PostgresTeamCliKeyRepository) Create(ctx context.Context, entity *clikey.TeamCliKeyEntity) error {
	if entity == nil {
		return errors.New("entity cannot be nil")
	}
	if entity.UUID == uuid.Nil {
		entity.UUID = uuid.New()
	}
	now := time.Now().UTC()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[entity.UUID] = entity
		return nil
	}

	configBytes := entity.Config
	if len(configBytes) == 0 {
		configBytes = []byte("{}")
	}

	query := `
		INSERT INTO team_cli_key (uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, config, "lastUsedAt", "expiresAt", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.pool.Exec(ctx, query,
		entity.UUID, entity.WorkspaceID, entity.TeamID, entity.Name, entity.KeyHash, entity.KeyPrefix,
		entity.Active, configBytes, entity.LastUsedAt, entity.ExpiresAt, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert team_cli_key: %w", err)
	}
	return nil
}

// FindByKeyPrefix finds an active key by its prefix.
func (r *PostgresTeamCliKeyRepository) FindByKeyPrefix(ctx context.Context, prefix string) (*clikey.TeamCliKeyEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, k := range r.memory {
			if k.KeyPrefix == prefix && k.Active {
				return k, nil
			}
		}
		return nil, nil
	}

	query := `
		SELECT uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, config, "lastUsedAt", "expiresAt", "createdAt", "updatedAt"
		FROM team_cli_key
		WHERE "keyPrefix" = $1 AND active = true
		LIMIT 1
	`
	var k clikey.TeamCliKeyEntity
	var configBytes []byte
	err := r.pool.QueryRow(ctx, query, prefix).Scan(
		&k.UUID, &k.WorkspaceID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix,
		&k.Active, &configBytes, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt,
	)
	if err != nil {
		return nil, nil
	}
	k.Config = configBytes
	return &k, nil
}

// ListByWorkspace returns all CLI keys for a workspace.
func (r *PostgresTeamCliKeyRepository) ListByWorkspace(ctx context.Context, wsID uuid.UUID) ([]*clikey.TeamCliKeyEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*clikey.TeamCliKeyEntity
		for _, k := range r.memory {
			if k.WorkspaceID == wsID {
				res = append(res, k)
			}
		}
		return res, nil
	}

	query := `
		SELECT uuid, workspace_id, team_id, name, "keyHash", "keyPrefix", active, config, "lastUsedAt", "expiresAt", "createdAt", "updatedAt"
		FROM team_cli_key
		WHERE workspace_id = $1
		ORDER BY "createdAt" DESC
	`
	rows, err := r.pool.Query(ctx, query, wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to query team_cli_key: %w", err)
	}
	defer rows.Close()

	var list []*clikey.TeamCliKeyEntity
	for rows.Next() {
		var k clikey.TeamCliKeyEntity
		var configBytes []byte
		if err := rows.Scan(
			&k.UUID, &k.WorkspaceID, &k.TeamID, &k.Name, &k.KeyHash, &k.KeyPrefix,
			&k.Active, &configBytes, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt,
		); err != nil {
			return nil, err
		}
		k.Config = configBytes
		list = append(list, &k)
	}
	return list, nil
}

// UpdateLastUsed sets the last used timestamp.
func (r *PostgresTeamCliKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if k, ok := r.memory[id]; ok {
			k.LastUsedAt = &now
			k.UpdatedAt = now
		}
		return nil
	}

	query := `UPDATE team_cli_key SET "lastUsedAt" = $1, "updatedAt" = $1 WHERE uuid = $2`
	_, err := r.pool.Exec(ctx, query, now, id)
	return err
}

// UpdateConfig updates the key's JSON configuration.
func (r *PostgresTeamCliKeyRepository) UpdateConfig(ctx context.Context, wsID, keyID uuid.UUID, config []byte) error {
	now := time.Now().UTC()
	if !json.Valid(config) {
		return errors.New("invalid JSON configuration")
	}

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if k, ok := r.memory[keyID]; ok && k.WorkspaceID == wsID {
			k.Config = config
			k.UpdatedAt = now
		}
		return nil
	}

	query := `UPDATE team_cli_key SET config = $1, "updatedAt" = $2 WHERE uuid = $3 AND workspace_id = $4`
	_, err := r.pool.Exec(ctx, query, config, now, keyID, wsID)
	return err
}

// Revoke deactivates a CLI key.
func (r *PostgresTeamCliKeyRepository) Revoke(ctx context.Context, wsID, keyID uuid.UUID) error {
	now := time.Now().UTC()
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if k, ok := r.memory[keyID]; ok && k.WorkspaceID == wsID {
			k.Active = false
			k.UpdatedAt = now
		}
		return nil
	}

	query := `UPDATE team_cli_key SET active = false, "updatedAt" = $1 WHERE uuid = $2 AND workspace_id = $3`
	_, err := r.pool.Exec(ctx, query, now, keyID, wsID)
	return err
}
