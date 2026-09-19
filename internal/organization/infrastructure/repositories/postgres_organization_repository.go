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
	orgdomain "github.com/scandrix/backend/internal/organization/domain/organization"
)

// PostgresOrganizationRepository implements IOrganizationRepository with PostgreSQL and in-memory fallback.
type PostgresOrganizationRepository struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	memory  map[uuid.UUID]*orgdomain.OrganizationEntity
}

// NewPostgresOrganizationRepository instantiates a new repository.
func NewPostgresOrganizationRepository(pool *pgxpool.Pool) *PostgresOrganizationRepository {
	return &PostgresOrganizationRepository{
		pool:   pool,
		memory: make(map[uuid.UUID]*orgdomain.OrganizationEntity),
	}
}

// Find retrieves organizations matching the specified filter.
func (r *PostgresOrganizationRepository) Find(ctx context.Context, filter orgdomain.OrganizationFilter) ([]*orgdomain.OrganizationEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		var res []*orgdomain.OrganizationEntity
		for _, org := range r.memory {
			if filter.UUID != nil && org.UUID != *filter.UUID {
				continue
			}
			if filter.Name != nil && org.Name != *filter.Name {
				continue
			}
			if filter.TenantName != nil && org.TenantName != *filter.TenantName {
				continue
			}
			if filter.Status != nil && org.Status != *filter.Status {
				continue
			}
			res = append(res, org)
		}
		return res, nil
	}

	query := `
		SELECT id, slug, name, status, created_at, updated_at
		FROM workspaces
		WHERE ($1::uuid IS NULL OR id = $1)
		  AND ($2::varchar IS NULL OR name = $2)
		  AND ($3::varchar IS NULL OR slug = $3)
		ORDER BY created_at DESC
	`
	var filterID *uuid.UUID
	if filter.UUID != nil {
		filterID = filter.UUID
	}
	rows, err := r.pool.Query(ctx, query, filterID, filter.Name, filter.TenantName)
	if err != nil {
		return nil, fmt.Errorf("failed to query workspaces: %w", err)
	}
	defer rows.Close()

	var list []*orgdomain.OrganizationEntity
	for rows.Next() {
		var id uuid.UUID
		var slug, name, statusStr string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&id, &slug, &name, &statusStr, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan workspace: %w", err)
		}
		list = append(list, &orgdomain.OrganizationEntity{
			UUID:         id,
			Name:         name,
			TenantName:   slug,
			Status:       statusStr == "ACTIVE",
			Language:     "en",
			Domain:       []string{},
			ReleaseTrack: orgdomain.DefaultReleaseTrack,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
		})
	}
	return list, nil
}

// FindOne returns a single organization matching the filter.
func (r *PostgresOrganizationRepository) FindOne(ctx context.Context, filter orgdomain.OrganizationFilter) (*orgdomain.OrganizationEntity, error) {
	list, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// FindByID retrieves an organization by its primary key UUID.
func (r *PostgresOrganizationRepository) FindByID(ctx context.Context, id uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid organization ID")
	}
	return r.FindOne(ctx, orgdomain.OrganizationFilter{UUID: &id})
}

// FindByUserID retrieves the organization associated with a user via the users table.
func (r *PostgresOrganizationRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*orgdomain.OrganizationEntity, error) {
	if userID == uuid.Nil {
		return nil, errors.New("invalid user ID")
	}

	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, org := range r.memory {
			return org, nil
		}
		return nil, nil
	}

	query := `
		SELECT w.id, w.name, w.slug, COALESCE(w.settings->>'status', 'ACTIVE'), w.created_at, w.updated_at
		FROM workspaces w
		JOIN users u ON (u.organization_id = w.id OR u.workspace_id = w.id)
		WHERE u.uuid = $1 OR u.id = $1
		LIMIT 1
	`
	var id uuid.UUID
	var name, slug, statusStr string
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, userID).Scan(&id, &name, &slug, &statusStr, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query organization by user ID: %w", err)
	}

	return &orgdomain.OrganizationEntity{
		UUID:         id,
		Name:         name,
		TenantName:   slug,
		Status:       statusStr == "ACTIVE",
		Language:     "en",
		Domain:       []string{},
		ReleaseTrack: orgdomain.DefaultReleaseTrack,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

// Create inserts a new organization record.
func (r *PostgresOrganizationRepository) Create(ctx context.Context, entity *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	if entity == nil {
		return nil, errors.New("organization entity cannot be nil")
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
		return entity, nil
	}

	query := `
		INSERT INTO workspaces (id, slug, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			slug = EXCLUDED.slug,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
		RETURNING id, slug, name, status, created_at, updated_at
	`
	statusStr := "ACTIVE"
	if !entity.Status {
		statusStr = "SUSPENDED"
	}

	var id uuid.UUID
	var slug, name, resStatus string
	var createdAt, updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, entity.UUID, entity.TenantName, entity.Name, statusStr, entity.CreatedAt, entity.UpdatedAt).
		Scan(&id, &slug, &name, &resStatus, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workspace: %w", err)
	}

	entity.UUID = id
	entity.TenantName = slug
	entity.Name = name
	entity.CreatedAt = createdAt
	entity.UpdatedAt = updatedAt
	return entity, nil
}

// DeleteOne removes an organization matching the filter.
func (r *PostgresOrganizationRepository) DeleteOne(ctx context.Context, filter orgdomain.OrganizationFilter) error {
	org, err := r.FindOne(ctx, filter)
	if err != nil || org == nil {
		return err
	}
	return r.Delete(ctx, org.UUID)
}

// Delete removes an organization by its ID.
func (r *PostgresOrganizationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.memory, id)
		return nil
	}

	query := `DELETE FROM workspaces WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}
	return nil
}

// Update updates an organization matching the filter.
func (r *PostgresOrganizationRepository) Update(ctx context.Context, filter orgdomain.OrganizationFilter, data *orgdomain.OrganizationEntity) (*orgdomain.OrganizationEntity, error) {
	existing, err := r.FindOne(ctx, filter)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("organization not found to update")
	}

	if data.Name != "" {
		existing.Name = data.Name
	}
	if data.TenantName != "" {
		existing.TenantName = data.TenantName
	}
	if data.Language != "" {
		existing.Language = data.Language
	}
	if len(data.Domain) > 0 {
		existing.Domain = data.Domain
	}
	if data.ReleaseTrack != "" {
		existing.ReleaseTrack = data.ReleaseTrack
	}
	existing.UpdatedAt = time.Now().UTC()

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[existing.UUID] = existing
		return existing, nil
	}

	query := `
		UPDATE workspaces
		SET name = $1, slug = $2, status = $3, updated_at = $4
		WHERE id = $5
	`
	statusStr := "ACTIVE"
	if !existing.Status {
		statusStr = "SUSPENDED"
	}

	_, err = r.pool.Exec(ctx, query, existing.Name, existing.TenantName, statusStr, existing.UpdatedAt, existing.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to update workspace: %w", err)
	}
	return existing, nil
}
