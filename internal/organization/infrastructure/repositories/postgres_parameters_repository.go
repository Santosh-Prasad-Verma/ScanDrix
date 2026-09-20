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
	paramdomain "github.com/scandrix/backend/internal/organization/domain/parameters"
)

// PostgresParametersRepository implements IParametersRepository.
type PostgresParametersRepository struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[string]*paramdomain.ParametersEntity // key: wsID:teamID:paramKey
}

// NewPostgresParametersRepository creates a new repository.
func NewPostgresParametersRepository(pool *pgxpool.Pool) *PostgresParametersRepository {
	return &PostgresParametersRepository{
		pool:   pool,
		memory: make(map[string]*paramdomain.ParametersEntity),
	}
}

func paramStorageKey(wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) string {
	tID := "nil"
	if teamID != nil {
		tID = teamID.String()
	}
	return fmt.Sprintf("%s:%s:%s", wsID.String(), tID, string(key))
}

// Find retrieves all parameters matching the filter.
func (r *PostgresParametersRepository) Find(ctx context.Context, filter paramdomain.ParametersFilter) ([]*paramdomain.ParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*paramdomain.ParametersEntity
		for _, p := range r.memory {
			if filter.UUID != nil && p.UUID != *filter.UUID {
				continue
			}
			if filter.WorkspaceID != nil && p.WorkspaceID != *filter.WorkspaceID {
				continue
			}
			if filter.TeamID != nil && (p.TeamID == nil || *p.TeamID != *filter.TeamID) {
				continue
			}
			if filter.ConfigKey != nil && p.ConfigKey != *filter.ConfigKey {
				continue
			}
			if filter.Active != nil && p.Active != *filter.Active {
				continue
			}
			res = append(res, p)
		}
		return res, nil
	}

	query := `
		SELECT id, workspace_id, team_id, config_key, config_value, description, active, version, created_at, updated_at
		FROM parameters
		WHERE ($1::uuid IS NULL OR id = $1)
		  AND ($2::uuid IS NULL OR workspace_id = $2)
		  AND ($3::uuid IS NULL OR team_id = $3)
		  AND ($4::varchar IS NULL OR config_key = $4)
		  AND ($5::boolean IS NULL OR active = $5)
		ORDER BY updated_at DESC
	`
	var filterID, filterWsID, filterTeamID *uuid.UUID
	var filterKey *string
	var filterActive *bool

	if filter.UUID != nil {
		filterID = filter.UUID
	}
	if filter.WorkspaceID != nil {
		filterWsID = filter.WorkspaceID
	}
	if filter.TeamID != nil {
		filterTeamID = filter.TeamID
	}
	if filter.ConfigKey != nil {
		k := string(*filter.ConfigKey)
		filterKey = &k
	}
	if filter.Active != nil {
		filterActive = filter.Active
	}

	rows, err := r.pool.Query(ctx, query, filterID, filterWsID, filterTeamID, filterKey, filterActive)
	if err != nil {
		// Fallback to legacy workspace_parameters if table parameters is not yet created
		return r.findLegacyWorkspaceParameters(ctx, filter)
	}
	defer rows.Close()

	var list []*paramdomain.ParametersEntity
	for rows.Next() {
		var id, wsID uuid.UUID
		var teamID *uuid.UUID
		var keyStr, desc string
		var configValRaw []byte
		var active bool
		var version int
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &wsID, &teamID, &keyStr, &configValRaw, &desc, &active, &version, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan parameter row: %w", err)
		}

		list = append(list, &paramdomain.ParametersEntity{
			UUID:        id,
			WorkspaceID: wsID,
			TeamID:      teamID,
			ConfigKey:   paramdomain.ParameterKey(keyStr),
			ConfigValue: json.RawMessage(configValRaw),
			Description: desc,
			Active:      active,
			Version:     version,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		})
	}

	return list, nil
}

func (r *PostgresParametersRepository) findLegacyWorkspaceParameters(ctx context.Context, filter paramdomain.ParametersFilter) ([]*paramdomain.ParametersEntity, error) {
	if filter.WorkspaceID == nil {
		return []*paramdomain.ParametersEntity{}, nil
	}

	query := `
		SELECT workspace_id, review_params, org_params, updated_at
		FROM workspace_parameters
		WHERE workspace_id = $1
	`
	var wsID uuid.UUID
	var reviewParamsRaw, orgParamsRaw []byte
	var updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, *filter.WorkspaceID).Scan(&wsID, &reviewParamsRaw, &orgParamsRaw, &updatedAt)
	if err != nil {
		return []*paramdomain.ParametersEntity{}, nil
	}

	var list []*paramdomain.ParametersEntity
	if filter.ConfigKey == nil || *filter.ConfigKey == paramdomain.KeyCodeReviewConfig {
		list = append(list, &paramdomain.ParametersEntity{
			UUID:        uuid.New(),
			WorkspaceID: wsID,
			ConfigKey:   paramdomain.KeyCodeReviewConfig,
			ConfigValue: json.RawMessage(reviewParamsRaw),
			Active:      true,
			UpdatedAt:   updatedAt,
		})
	}
	if filter.ConfigKey == nil || *filter.ConfigKey == paramdomain.KeyPlatformConfigs {
		list = append(list, &paramdomain.ParametersEntity{
			UUID:        uuid.New(),
			WorkspaceID: wsID,
			ConfigKey:   paramdomain.KeyPlatformConfigs,
			ConfigValue: json.RawMessage(orgParamsRaw),
			Active:      true,
			UpdatedAt:   updatedAt,
		})
	}

	return list, nil
}

// FindOne returns a single parameter matching the filter.
func (r *PostgresParametersRepository) FindOne(ctx context.Context, filter paramdomain.ParametersFilter) (*paramdomain.ParametersEntity, error) {
	list, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// FindByID retrieves a parameter by UUID.
func (r *PostgresParametersRepository) FindByID(ctx context.Context, id uuid.UUID) (*paramdomain.ParametersEntity, error) {
	return r.FindOne(ctx, paramdomain.ParametersFilter{UUID: &id})
}

// FindByKey retrieves a parameter by workspace, team, and key.
func (r *PostgresParametersRepository) FindByKey(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) (*paramdomain.ParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memory[paramStorageKey(wsID, teamID, key)]; ok {
			return p, nil
		}
		return nil, nil
	}

	active := true
	return r.FindOne(ctx, paramdomain.ParametersFilter{
		WorkspaceID: &wsID,
		TeamID:      teamID,
		ConfigKey:   &key,
		Active:      &active,
	})
}

// Create stores or updates a parameter.
func (r *PostgresParametersRepository) Create(ctx context.Context, entity *paramdomain.ParametersEntity) (*paramdomain.ParametersEntity, error) {
	if entity == nil {
		return nil, errors.New("parameters entity cannot be nil")
	}
	if entity.UUID == uuid.Nil {
		entity.UUID = uuid.New()
	}
	now := time.Now().UTC()
	if entity.CreatedAt.IsZero() {
		entity.CreatedAt = now
	}
	entity.UpdatedAt = now
	if entity.Version == 0 {
		entity.Version = 1
	}

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[paramStorageKey(entity.WorkspaceID, entity.TeamID, entity.ConfigKey)] = entity
		return entity, nil
	}

	valBytes, err := json.Marshal(entity.ConfigValue)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameter value: %w", err)
	}

	query := `
		INSERT INTO parameters (id, workspace_id, team_id, config_key, config_value, description, active, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (workspace_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid), config_key) WHERE active = true
		DO UPDATE SET
			config_value = EXCLUDED.config_value,
			description = EXCLUDED.description,
			version = parameters.version + 1,
			updated_at = EXCLUDED.updated_at
		RETURNING id, version, updated_at
	`
	err = r.pool.QueryRow(ctx, query,
		entity.UUID,
		entity.WorkspaceID,
		entity.TeamID,
		string(entity.ConfigKey),
		valBytes,
		entity.Description,
		entity.Active,
		entity.Version,
		entity.CreatedAt,
		entity.UpdatedAt,
	).Scan(&entity.UUID, &entity.Version, &entity.UpdatedAt)

	if err != nil {
		// Fallback to legacy workspace_parameters if table parameters is not yet migrated
		return r.createLegacyWorkspaceParameters(ctx, entity, valBytes)
	}

	return entity, nil
}

func (r *PostgresParametersRepository) createLegacyWorkspaceParameters(ctx context.Context, entity *paramdomain.ParametersEntity, valBytes []byte) (*paramdomain.ParametersEntity, error) {
	if entity.ConfigKey == paramdomain.KeyCodeReviewConfig {
		query := `
			INSERT INTO workspace_parameters (workspace_id, review_params, updated_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (workspace_id) DO UPDATE SET
				review_params = EXCLUDED.review_params,
				updated_at = EXCLUDED.updated_at
		`
		_, err := r.pool.Exec(ctx, query, entity.WorkspaceID, valBytes, entity.UpdatedAt)
		if err != nil {
			return nil, err
		}
	} else {
		query := `
			INSERT INTO workspace_parameters (workspace_id, org_params, updated_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (workspace_id) DO UPDATE SET
				org_params = EXCLUDED.org_params,
				updated_at = EXCLUDED.updated_at
		`
		_, err := r.pool.Exec(ctx, query, entity.WorkspaceID, valBytes, entity.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	return entity, nil
}

// Update modifies an existing parameter.
func (r *PostgresParametersRepository) Update(ctx context.Context, filter paramdomain.ParametersFilter, data *paramdomain.ParametersEntity) (*paramdomain.ParametersEntity, error) {
	existing, err := r.FindOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("parameter not found to update")
	}

	existing.ConfigValue = data.ConfigValue
	existing.Active = data.Active
	existing.Description = data.Description
	existing.UpdatedAt = time.Now().UTC()

	return r.Create(ctx, existing)
}

// Delete removes or deactivates a parameter by key.
func (r *PostgresParametersRepository) Delete(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.memory, paramStorageKey(wsID, teamID, key))
		return nil
	}

	query := `
		UPDATE parameters 
		SET active = false, updated_at = now() 
		WHERE workspace_id = $1 
		  AND ($2::uuid IS NULL OR team_id = $2)
		  AND config_key = $3
	`
	var filterTeamID *uuid.UUID
	if teamID != nil {
		filterTeamID = teamID
	}
	_, err := r.pool.Exec(ctx, query, wsID, filterTeamID, string(key))
	if err != nil {
		// Fallback
		if key == paramdomain.KeyCodeReviewConfig {
			_, _ = r.pool.Exec(ctx, `UPDATE workspace_parameters SET review_params = '{}'::jsonb, updated_at = now() WHERE workspace_id = $1`, wsID)
		} else {
			_, _ = r.pool.Exec(ctx, `UPDATE workspace_parameters SET org_params = '{}'::jsonb, updated_at = now() WHERE workspace_id = $1`, wsID)
		}
	}
	return nil
}

// DeleteByTeamID purges parameters associated with a deleted team.
func (r *PostgresParametersRepository) DeleteByTeamID(ctx context.Context, teamID uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		for k, p := range r.memory {
			if p.TeamID != nil && *p.TeamID == teamID {
				delete(r.memory, k)
			}
		}
		return nil
	}

	query := `UPDATE parameters SET active = false, updated_at = now() WHERE team_id = $1`
	_, err := r.pool.Exec(ctx, query, teamID)
	return err
}

// CreateNewActiveVersion atomically deactivates every currently-active row for (wsID, teamID, key)
// and inserts a brand-new active row with version = nextVersion.
func (r *PostgresParametersRepository) CreateNewActiveVersion(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any, nextVersion int) (*paramdomain.ParametersEntity, error) {
	valBytes, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameter value: %w", err)
	}

	newUUID := uuid.New()
	now := time.Now().UTC()

	entity := &paramdomain.ParametersEntity{
		UUID:        newUUID,
		WorkspaceID: wsID,
		TeamID:      teamID,
		ConfigKey:   key,
		ConfigValue: valBytes,
		Active:      true,
		Version:     nextVersion,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[paramStorageKey(wsID, teamID, key)] = entity
		return entity, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Bulk-deactivate all currently active rows for this (workspace_id, team_id, config_key)
	deactivateQuery := `
		UPDATE parameters
		SET active = false, updated_at = $4
		WHERE workspace_id = $1
		  AND (team_id IS NOT DISTINCT FROM $2)
		  AND config_key = $3
		  AND active = true
	`
	_, err = tx.Exec(ctx, deactivateQuery, wsID, teamID, string(key), now)
	if err != nil {
		return nil, fmt.Errorf("failed deactivating prior active versions: %w", err)
	}

	// 2. Insert the new active version
	insertQuery := `
		INSERT INTO parameters (id, workspace_id, team_id, config_key, config_value, active, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, $6, $7, $8)
	`
	_, err = tx.Exec(ctx, insertQuery, newUUID, wsID, teamID, string(key), valBytes, nextVersion, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed inserting new active version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed committing new active version: %w", err)
	}

	return entity, nil
}

// CreateActiveVersionIfAbsent inserts the first active version only when none exists.
func (r *PostgresParametersRepository) CreateActiveVersionIfAbsent(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, key paramdomain.ParameterKey, val any) (*paramdomain.ParametersEntity, error) {
	existing, err := r.FindByKey(ctx, wsID, teamID, key)
	if err == nil && existing != nil {
		return existing, nil
	}
	return r.CreateNewActiveVersion(ctx, wsID, teamID, key, val, 1)
}
