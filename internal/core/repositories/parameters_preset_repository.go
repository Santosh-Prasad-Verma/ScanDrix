package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgGlobalParametersRepository manages system-wide dynamic flags and global parameters.
type PgGlobalParametersRepository struct {
	pool *pgxpool.Pool
}

// NewGlobalParametersRepository instantiates a new repository.
func NewGlobalParametersRepository(pool *pgxpool.Pool) *PgGlobalParametersRepository {
	return &PgGlobalParametersRepository{pool: pool}
}

// Get retrieves a global parameter by its unique key.
func (r *PgGlobalParametersRepository) Get(ctx context.Context, paramKey string) (*domain.GlobalParameters, error) {
	query := `
		SELECT id, created_at, updated_at, param_key, param_value, value_type, description, is_public, requires_restart
		FROM global_parameters
		WHERE param_key = $1
		LIMIT 1
	`
	gp := &domain.GlobalParameters{}
	err := r.pool.QueryRow(ctx, query, paramKey).Scan(
		&gp.ID, &gp.CreatedAt, &gp.UpdatedAt, &gp.ParamKey, &gp.ParamValue,
		&gp.ValueType, &gp.Description, &gp.IsPublic, &gp.RequiresRestart,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying global parameter: %w", err)
	}
	return gp, nil
}

// Set creates or updates a global parameter value.
func (r *PgGlobalParametersRepository) Set(ctx context.Context, gp *domain.GlobalParameters) error {
	query := `
		INSERT INTO global_parameters (
			id, created_at, updated_at, param_key, param_value, value_type, description, is_public, requires_restart
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (param_key) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    param_value = EXCLUDED.param_value,
		    value_type = EXCLUDED.value_type,
		    description = EXCLUDED.description,
		    is_public = EXCLUDED.is_public,
		    requires_restart = EXCLUDED.requires_restart
	`
	now := time.Now().UTC()
	if gp.ID == uuid.Nil {
		gp.ID = uuid.New()
	}
	gp.CreatedAt = now
	gp.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		gp.ID, gp.CreatedAt, gp.UpdatedAt, gp.ParamKey, gp.ParamValue,
		gp.ValueType, gp.Description, gp.IsPublic, gp.RequiresRestart,
	)
	if err != nil {
		return fmt.Errorf("failed upserting global parameter: %w", err)
	}
	return nil
}

// ListAll returns all global system parameters.
func (r *PgGlobalParametersRepository) ListAll(ctx context.Context) ([]*domain.GlobalParameters, error) {
	query := `
		SELECT id, created_at, updated_at, param_key, param_value, value_type, description, is_public, requires_restart
		FROM global_parameters
		ORDER BY param_key ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed listing global parameters: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.GlobalParameters, 0)
	for rows.Next() {
		gp := &domain.GlobalParameters{}
		if err := rows.Scan(
			&gp.ID, &gp.CreatedAt, &gp.UpdatedAt, &gp.ParamKey, &gp.ParamValue,
			&gp.ValueType, &gp.Description, &gp.IsPublic, &gp.RequiresRestart,
		); err != nil {
			return nil, fmt.Errorf("failed scanning global parameter: %w", err)
		}
		items = append(items, gp)
	}
	return items, nil
}

// PgParametersPresetRepository manages pre-curated rule configurations.
type PgParametersPresetRepository struct {
	pool *pgxpool.Pool
}

// NewParametersPresetRepository instantiates a new repository.
func NewParametersPresetRepository(pool *pgxpool.Pool) *PgParametersPresetRepository {
	return &PgParametersPresetRepository{pool: pool}
}

// FindBySlug retrieves a preset by its slug (e.g. "strict-security", "owasp-top-10").
func (r *PgParametersPresetRepository) FindBySlug(ctx context.Context, slug string) (*domain.ParametersPreset, error) {
	query := `
		SELECT id, created_at, updated_at, slug, name, description, category, configuration, is_default
		FROM parameters_presets
		WHERE slug = $1
		LIMIT 1
	`
	p := &domain.ParametersPreset{}
	err := r.pool.QueryRow(ctx, query, slug).Scan(
		&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Slug, &p.Name,
		&p.Description, &p.Category, &p.Configuration, &p.IsDefault,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying parameters preset: %w", err)
	}
	return p, nil
}

// ListAll returns all available parameter presets.
func (r *PgParametersPresetRepository) ListAll(ctx context.Context) ([]*domain.ParametersPreset, error) {
	query := `
		SELECT id, created_at, updated_at, slug, name, description, category, configuration, is_default
		FROM parameters_presets
		ORDER BY category ASC, name ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed listing parameters presets: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.ParametersPreset, 0)
	for rows.Next() {
		p := &domain.ParametersPreset{}
		if err := rows.Scan(
			&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Slug, &p.Name,
			&p.Description, &p.Category, &p.Configuration, &p.IsDefault,
		); err != nil {
			return nil, fmt.Errorf("failed scanning parameters preset: %w", err)
		}
		items = append(items, p)
	}
	return items, nil
}
