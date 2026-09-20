// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package repositories

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	globalparamdomain "github.com/scandrix/backend/internal/organization/domain/globalparameters"
)

// PostgresGlobalParametersRepository implements IGlobalParametersRepository.
type PostgresGlobalParametersRepository struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[string]*globalparamdomain.GlobalParametersEntity // key: config_key
}

// NewPostgresGlobalParametersRepository creates a new repository.
func NewPostgresGlobalParametersRepository(pool *pgxpool.Pool) *PostgresGlobalParametersRepository {
	return &PostgresGlobalParametersRepository{
		pool:   pool,
		memory: make(map[string]*globalparamdomain.GlobalParametersEntity),
	}
}

// FindByID retrieves a global parameter by primary UUID.
func (r *PostgresGlobalParametersRepository) FindByID(ctx context.Context, id uuid.UUID) (*globalparamdomain.GlobalParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, p := range r.memory {
			if p.UUID == id {
				return p, nil
			}
		}
		return nil, nil
	}

	query := `
		SELECT id, config_key, config_value, description, created_at, updated_at
		FROM global_parameters
		WHERE id = $1
		LIMIT 1
	`
	var p globalparamdomain.GlobalParametersEntity
	var valBytes []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(&p.UUID, &p.ConfigKey, &valBytes, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query global parameter by id: %w", err)
	}
	p.ConfigValue = valBytes
	return &p, nil
}

// FindByKey retrieves a global parameter by config_key.
func (r *PostgresGlobalParametersRepository) FindByKey(ctx context.Context, key string) (*globalparamdomain.GlobalParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memory[key]; ok {
			return p, nil
		}
		return nil, nil
	}

	query := `
		SELECT id, config_key, config_value, description, created_at, updated_at
		FROM global_parameters
		WHERE config_key = $1
		LIMIT 1
	`
	var p globalparamdomain.GlobalParametersEntity
	var valBytes []byte
	err := r.pool.QueryRow(ctx, query, key).Scan(&p.UUID, &p.ConfigKey, &valBytes, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query global parameter by key: %w", err)
	}
	p.ConfigValue = valBytes
	return &p, nil
}

// FindUpdatedAtByKey returns the last updated timestamp for a config_key.
func (r *PostgresGlobalParametersRepository) FindUpdatedAtByKey(ctx context.Context, key string) (*time.Time, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if p, ok := r.memory[key]; ok {
			return &p.UpdatedAt, nil
		}
		return nil, nil
	}

	query := `
		SELECT updated_at
		FROM global_parameters
		WHERE config_key = $1
		LIMIT 1
	`
	var updatedAt time.Time
	err := r.pool.QueryRow(ctx, query, key).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query global parameter updated_at: %w", err)
	}
	return &updatedAt, nil
}

// Delete removes a global parameter by UUID.
func (r *PostgresGlobalParametersRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		for k, p := range r.memory {
			if p.UUID == id {
				delete(r.memory, k)
				break
			}
		}
		return nil
	}

	query := `DELETE FROM global_parameters WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// Create inserts or upserts a global parameter.
func (r *PostgresGlobalParametersRepository) Create(ctx context.Context, entity *globalparamdomain.GlobalParametersEntity) error {
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
		r.memory[entity.ConfigKey] = entity
		return nil
	}

	query := `
		INSERT INTO global_parameters (id, config_key, config_value, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (config_key) DO UPDATE SET
			config_value = EXCLUDED.config_value,
			description = EXCLUDED.description,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.pool.Exec(ctx, query, entity.UUID, entity.ConfigKey, entity.ConfigValue, entity.Description, entity.CreatedAt, entity.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert global parameter: %w", err)
	}
	return nil
}

// Update updates an existing parameter.
func (r *PostgresGlobalParametersRepository) Update(ctx context.Context, entity *globalparamdomain.GlobalParametersEntity) error {
	return r.Create(ctx, entity)
}

// List retrieves all global parameters.
func (r *PostgresGlobalParametersRepository) List(ctx context.Context) ([]*globalparamdomain.GlobalParametersEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var list []*globalparamdomain.GlobalParametersEntity
		for _, p := range r.memory {
			list = append(list, p)
		}
		return list, nil
	}

	query := `
		SELECT id, config_key, config_value, description, created_at, updated_at
		FROM global_parameters
		ORDER BY config_key ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list global parameters: %w", err)
	}
	defer rows.Close()

	var list []*globalparamdomain.GlobalParametersEntity
	for rows.Next() {
		var p globalparamdomain.GlobalParametersEntity
		var valBytes []byte
		if err := rows.Scan(&p.UUID, &p.ConfigKey, &valBytes, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ConfigValue = valBytes
		list = append(list, &p)
	}
	return list, nil
}
