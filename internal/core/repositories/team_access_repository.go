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

// PgTeamRepository manages engineering squad entities.
type PgTeamRepository struct {
	pool *pgxpool.Pool
}

// NewTeamRepository instantiates a new PgTeamRepository.
func NewTeamRepository(pool *pgxpool.Pool) *PgTeamRepository {
	return &PgTeamRepository{pool: pool}
}

// FindByID retrieves a team by its UUID within a workspace.
func (r *PgTeamRepository) FindByID(ctx context.Context, wsID, teamID uuid.UUID) (*domain.Team, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, slug, name, description, is_active, settings
		FROM teams
		WHERE workspace_id = $1 AND id = $2
	`
	t := &domain.Team{}
	err := r.pool.QueryRow(ctx, query, wsID, teamID).Scan(
		&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.WorkspaceID, &t.OrganizationID,
		&t.Slug, &t.Name, &t.Description, &t.IsActive, &t.Settings,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying team: %w", err)
	}
	return t, nil
}

// Create inserts a new team.
func (r *PgTeamRepository) Create(ctx context.Context, t *domain.Team) error {
	query := `
		INSERT INTO teams (
			id, created_at, updated_at, workspace_id, organization_id, slug, name, description, is_active, settings
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	now := time.Now().UTC()
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	t.CreatedAt = now
	t.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		t.ID, t.CreatedAt, t.UpdatedAt, t.WorkspaceID, t.OrganizationID,
		t.Slug, t.Name, t.Description, t.IsActive, t.Settings,
	)
	if err != nil {
		return fmt.Errorf("failed creating team: %w", err)
	}
	return nil
}

// ListByOrganization returns all active teams in an organization.
func (r *PgTeamRepository) ListByOrganization(ctx context.Context, wsID, orgID uuid.UUID) ([]*domain.Team, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, organization_id, slug, name, description, is_active, settings
		FROM teams
		WHERE workspace_id = $1 AND organization_id = $2
		ORDER BY name ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed listing organization teams: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.Team, 0)
	for rows.Next() {
		t := &domain.Team{}
		if err := rows.Scan(
			&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.WorkspaceID, &t.OrganizationID,
			&t.Slug, &t.Name, &t.Description, &t.IsActive, &t.Settings,
		); err != nil {
			return nil, fmt.Errorf("failed scanning team row: %w", err)
		}
		items = append(items, t)
	}
	return items, nil
}

// PgTeamCliKeyRepository manages API tokens used by developers for local code review.
type PgTeamCliKeyRepository struct {
	pool *pgxpool.Pool
}

// NewTeamCliKeyRepository instantiates a new repository.
func NewTeamCliKeyRepository(pool *pgxpool.Pool) *PgTeamCliKeyRepository {
	return &PgTeamCliKeyRepository{pool: pool}
}

// Create records a newly provisioned team CLI API key.
func (r *PgTeamCliKeyRepository) Create(ctx context.Context, key *domain.TeamCliKey) error {
	query := `
		INSERT INTO team_cli_keys (
			id, created_at, updated_at, workspace_id, team_id, created_by_user_id,
			key_prefix, key_hash, name, scopes, last_used_at, expires_at, revoked_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	now := time.Now().UTC()
	if key.ID == uuid.Nil {
		key.ID = uuid.New()
	}
	key.CreatedAt = now
	key.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		key.ID, key.CreatedAt, key.UpdatedAt, key.WorkspaceID, key.TeamID, key.CreatedByUserID,
		key.KeyPrefix, key.KeyHash, key.Name, key.Scopes, key.LastUsedAt, key.ExpiresAt, key.RevokedAt,
	)
	if err != nil {
		return fmt.Errorf("failed inserting team cli key: %w", err)
	}
	return nil
}

// FindByHash locates a valid, unrevoked key by its cryptographic SHA-256 hash.
func (r *PgTeamCliKeyRepository) FindByHash(ctx context.Context, keyHash string) (*domain.TeamCliKey, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, team_id, created_by_user_id,
		       key_prefix, key_hash, name, scopes, last_used_at, expires_at, revoked_at
		FROM team_cli_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > now())
		LIMIT 1
	`
	k := &domain.TeamCliKey{}
	err := r.pool.QueryRow(ctx, query, keyHash).Scan(
		&k.ID, &k.CreatedAt, &k.UpdatedAt, &k.WorkspaceID, &k.TeamID, &k.CreatedByUserID,
		&k.KeyPrefix, &k.KeyHash, &k.Name, &k.Scopes, &k.LastUsedAt, &k.ExpiresAt, &k.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed querying team cli key by hash: %w", err)
	}
	return k, nil
}

// UpdateLastUsed updates the access timestamp of a team CLI key.
func (r *PgTeamCliKeyRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE team_cli_keys
		SET last_used_at = $2, updated_at = $2
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id, time.Now().UTC())
	return err
}

// Revoke invalidates an active API key immediately.
func (r *PgTeamCliKeyRepository) Revoke(ctx context.Context, wsID, id uuid.UUID) error {
	query := `
		UPDATE team_cli_keys
		SET revoked_at = $3, updated_at = $3
		WHERE workspace_id = $1 AND id = $2 AND revoked_at IS NULL
	`
	cmd, err := r.pool.Exec(ctx, query, wsID, id, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed revoking cli key: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PgCliDeviceRepository tracks developer workstations running local review engines.
type PgCliDeviceRepository struct {
	pool *pgxpool.Pool
}

// NewCliDeviceRepository instantiates a new PgCliDeviceRepository.
func NewCliDeviceRepository(pool *pgxpool.Pool) *PgCliDeviceRepository {
	return &PgCliDeviceRepository{pool: pool}
}

// RegisterOrHeartbeat records a device check-in or creates a new registration.
func (r *PgCliDeviceRepository) RegisterOrHeartbeat(ctx context.Context, dev *domain.CliDevice) error {
	query := `
		INSERT INTO cli_devices (
			id, created_at, updated_at, workspace_id, user_id, device_identifier,
			hostname, os, arch, client_version, last_seen_at, is_revoked
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (workspace_id, device_identifier) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    hostname = EXCLUDED.hostname,
		    client_version = EXCLUDED.client_version,
		    last_seen_at = EXCLUDED.last_seen_at
	`
	now := time.Now().UTC()
	if dev.ID == uuid.Nil {
		dev.ID = uuid.New()
	}
	dev.CreatedAt = now
	dev.UpdatedAt = now
	dev.LastSeenAt = now

	_, err := r.pool.Exec(ctx, query,
		dev.ID, dev.CreatedAt, dev.UpdatedAt, dev.WorkspaceID, dev.UserID, dev.DeviceIdentifier,
		dev.Hostname, dev.OS, dev.Arch, dev.ClientVersion, dev.LastSeenAt, dev.IsRevoked,
	)
	if err != nil {
		return fmt.Errorf("failed recording cli device: %w", err)
	}
	return nil
}
