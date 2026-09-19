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
	orgparamdomain "github.com/scandrix/backend/internal/organization/domain/organizationparameters"
)

// PostgresOrganizationParametersRepository implements IOrganizationParametersRepository with PostgreSQL and in-memory fallback.
type PostgresOrganizationParametersRepository struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[string]*orgparamdomain.OrganizationParametersEntity // key: wsID:configKey
}

// NewPostgresOrganizationParametersRepository instantiates a new repository.
func NewPostgresOrganizationParametersRepository(pool *pgxpool.Pool) *PostgresOrganizationParametersRepository {
	return &PostgresOrganizationParametersRepository{
		pool:   pool,
		memory: make(map[string]*orgparamdomain.OrganizationParametersEntity),
	}
}

func orgParamKey(wsID uuid.UUID, key orgparamdomain.ParameterKey) string {
	return fmt.Sprintf("%s:%s", wsID.String(), string(key))
}

// Find retrieves organization parameters matching the specified filter.
func (r *PostgresOrganizationParametersRepository) Find(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*orgparamdomain.OrganizationParametersEntity
		for _, param := range r.memory {
			if filter.UUID != nil && param.UUID != *filter.UUID {
				continue
			}
			if filter.WorkspaceID != nil && param.WorkspaceID != *filter.WorkspaceID {
				continue
			}
			if filter.ConfigKey != nil && param.ConfigKey != *filter.ConfigKey {
				continue
			}
			if filter.IsActive != nil && param.IsActive != *filter.IsActive {
				continue
			}
			res = append(res, param)
		}
		return res, nil
	}

	query := `
		SELECT id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at
		FROM organization_parameters
		WHERE ($1::uuid IS NULL OR id = $1)
		  AND ($2::uuid IS NULL OR workspace_id = $2)
		  AND ($3::varchar IS NULL OR config_key = $3)
		  AND ($4::boolean IS NULL OR is_active = $4)
		ORDER BY created_at ASC
	`
	var filterID, filterWsID *uuid.UUID
	var keyStr *string
	if filter.UUID != nil {
		filterID = filter.UUID
	}
	if filter.WorkspaceID != nil {
		filterWsID = filter.WorkspaceID
	}
	if filter.ConfigKey != nil {
		k := string(*filter.ConfigKey)
		keyStr = &k
	}

	rows, err := r.pool.Query(ctx, query, filterID, filterWsID, keyStr, filter.IsActive)
	if err != nil {
		return nil, fmt.Errorf("failed to query organization_parameters: %w", err)
	}
	defer rows.Close()

	var list []*orgparamdomain.OrganizationParametersEntity
	for rows.Next() {
		var id, wsID uuid.UUID
		var configKey, description string
		var configValueRaw []byte
		var isActive bool
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &wsID, &configKey, &configValueRaw, &description, &isActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan organization_parameters: %w", err)
		}

		list = append(list, &orgparamdomain.OrganizationParametersEntity{
			UUID:        id,
			WorkspaceID: wsID,
			ConfigKey:   orgparamdomain.ParameterKey(configKey),
			ConfigValue: configValueRaw,
			Description: description,
			IsActive:    isActive,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		})
	}
	return list, nil
}

// FindOne returns a single organization parameter matching the filter.
func (r *PostgresOrganizationParametersRepository) FindOne(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter) (*orgparamdomain.OrganizationParametersEntity, error) {
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
func (r *PostgresOrganizationParametersRepository) FindByID(ctx context.Context, id uuid.UUID) (*orgparamdomain.OrganizationParametersEntity, error) {
	return r.FindOne(ctx, orgparamdomain.OrganizationParametersFilter{UUID: &id})
}

// FindByKey retrieves an active configuration parameter by workspace ID and key.
func (r *PostgresOrganizationParametersRepository) FindByKey(ctx context.Context, wsID uuid.UUID, key orgparamdomain.ParameterKey) (*orgparamdomain.OrganizationParametersEntity, error) {
	if wsID == uuid.Nil || key == "" {
		return nil, errors.New("workspace ID and config key are required")
	}
	isActive := true
	return r.FindOne(ctx, orgparamdomain.OrganizationParametersFilter{
		WorkspaceID: &wsID,
		ConfigKey:   &key,
		IsActive:    &isActive,
	})
}

// FindByOrganizationName retrieves a parameter entity by matching organizationName in JSON config.
func (r *PostgresOrganizationParametersRepository) FindByOrganizationName(ctx context.Context, orgName string) (*orgparamdomain.OrganizationParametersEntity, error) {
	if orgName == "" {
		return nil, errors.New("organization name cannot be empty")
	}
	match := map[string]any{"organizationName": orgName}
	jsonBytes, err := json.Marshal(match)
	if err != nil {
		return nil, err
	}

	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, p := range r.memory {
			if p.IsActive {
				var raw map[string]any
				if err := json.Unmarshal(p.ConfigValue, &raw); err == nil {
					if name, ok := raw["organizationName"].(string); ok && name == orgName {
						return p, nil
					}
				}
			}
		}
		return nil, nil
	}

	query := `
		SELECT id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at
		FROM organization_parameters
		WHERE is_active = true AND config_value @> $1::jsonb
		LIMIT 1
	`
	var id, wsID uuid.UUID
	var configKey, description string
	var configValueRaw []byte
	var isActive bool
	var createdAt, updatedAt time.Time

	err = r.pool.QueryRow(ctx, query, jsonBytes).Scan(&id, &wsID, &configKey, &configValueRaw, &description, &isActive, &createdAt, &updatedAt)
	if err != nil {
		return nil, nil
	}

	return &orgparamdomain.OrganizationParametersEntity{
		UUID:        id,
		WorkspaceID: wsID,
		ConfigKey:   orgparamdomain.ParameterKey(configKey),
		ConfigValue: configValueRaw,
		Description: description,
		IsActive:    isActive,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

// FindByKeyAndValue finds all parameters matching a JSON subset using PostgreSQL GIN index.
func (r *PostgresOrganizationParametersRepository) FindByKeyAndValue(ctx context.Context, key orgparamdomain.ParameterKey, matchJSON map[string]any) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	return r.FindByKeyAndValueFuzzy(ctx, key, matchJSON, true)
}

// FindByKeyAndValueFuzzy finds parameters matching JSON with optional exact or subset containment.
func (r *PostgresOrganizationParametersRepository) FindByKeyAndValueFuzzy(ctx context.Context, key orgparamdomain.ParameterKey, matchJSON map[string]any, fuzzy bool) ([]*orgparamdomain.OrganizationParametersEntity, error) {
	jsonBytes, err := json.Marshal(matchJSON)
	if err != nil {
		return nil, err
	}

	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*orgparamdomain.OrganizationParametersEntity
		for _, p := range r.memory {
			if p.ConfigKey == key && p.IsActive {
				res = append(res, p)
			}
		}
		return res, nil
	}

	operator := "@>"
	if !fuzzy {
		operator = "="
	}
	query := fmt.Sprintf(`
		SELECT id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at
		FROM organization_parameters
		WHERE config_key = $1 AND is_active = true AND config_value %s $2::jsonb
	`, operator)
	rows, err := r.pool.Query(ctx, query, string(key), jsonBytes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*orgparamdomain.OrganizationParametersEntity
	for rows.Next() {
		var id, wsID uuid.UUID
		var configKey, description string
		var configValueRaw []byte
		var isActive bool
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &wsID, &configKey, &configValueRaw, &description, &isActive, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		list = append(list, &orgparamdomain.OrganizationParametersEntity{
			UUID:        id,
			WorkspaceID: wsID,
			ConfigKey:   orgparamdomain.ParameterKey(configKey),
			ConfigValue: configValueRaw,
			Description: description,
			IsActive:    isActive,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		})
	}
	return list, nil
}

// Create inserts or upserts an organization parameter.
func (r *PostgresOrganizationParametersRepository) Create(ctx context.Context, entity *orgparamdomain.OrganizationParametersEntity) (*orgparamdomain.OrganizationParametersEntity, error) {
	if entity == nil {
		return nil, errors.New("entity cannot be nil")
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
		r.memory[orgParamKey(entity.WorkspaceID, entity.ConfigKey)] = entity
		return entity, nil
	}

	query := `
		INSERT INTO organization_parameters (id, workspace_id, config_key, config_value, description, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (workspace_id, config_key) DO UPDATE SET
			config_value = EXCLUDED.config_value,
			description = EXCLUDED.description,
			is_active = EXCLUDED.is_active,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`

	var id uuid.UUID
	var createdAt, updatedAt time.Time
	err := r.pool.QueryRow(ctx, query,
		entity.UUID, entity.WorkspaceID, string(entity.ConfigKey), entity.ConfigValue, entity.Description, entity.IsActive, entity.CreatedAt, entity.UpdatedAt,
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert organization_parameters: %w", err)
	}

	entity.UUID = id
	entity.CreatedAt = createdAt
	entity.UpdatedAt = updatedAt
	return entity, nil
}

// Update updates an organization parameter matching the filter.
func (r *PostgresOrganizationParametersRepository) Update(ctx context.Context, filter orgparamdomain.OrganizationParametersFilter, data *orgparamdomain.OrganizationParametersEntity) (*orgparamdomain.OrganizationParametersEntity, error) {
	existing, err := r.FindOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("organization parameter not found")
	}

	existing.ConfigValue = data.ConfigValue
	existing.Description = data.Description
	existing.IsActive = data.IsActive
	existing.UpdatedAt = time.Now().UTC()

	return r.Create(ctx, existing)
}

// Delete removes a parameter by workspace ID and key.
func (r *PostgresOrganizationParametersRepository) Delete(ctx context.Context, wsID uuid.UUID, key orgparamdomain.ParameterKey) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.memory, orgParamKey(wsID, key))
		return nil
	}

	query := `DELETE FROM organization_parameters WHERE workspace_id = $1 AND config_key = $2`
	_, err := r.pool.Exec(ctx, query, wsID, string(key))
	if err != nil {
		return fmt.Errorf("failed to delete organization parameter: %w", err)
	}
	return nil
}
