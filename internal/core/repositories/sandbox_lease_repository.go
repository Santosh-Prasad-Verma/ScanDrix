package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// SandboxLease models an isolated execution environment lease for code reviews.
type SandboxLease struct {
	domain.TenantScopedEntity
	ReviewID        uuid.UUID  `json:"review_id" db:"review_id"`
	SandboxProvider string     `json:"sandbox_provider" db:"sandbox_provider"`
	ExternalLeaseID string     `json:"external_lease_id" db:"external_lease_id"`
	IPAddress       *string    `json:"ip_address,omitempty" db:"ip_address"`
	Port            int        `json:"port" db:"port"`
	Status          string     `json:"status" db:"status"` // "PROVISIONING", "ACTIVE", "RELEASED", "EXPIRED"
	ExpiresAt       time.Time  `json:"expires_at" db:"expires_at"`
	ReleasedAt      *time.Time `json:"released_at,omitempty" db:"released_at"`
}

// SandboxLeaseRepository defines persistence operations for ephemeral sandbox leases.
type SandboxLeaseRepository interface {
	AcquireLease(ctx context.Context, lease *SandboxLease) error
	FindByReviewID(ctx context.Context, wsID, reviewID uuid.UUID) (*SandboxLease, error)
	ReleaseLease(ctx context.Context, wsID, id uuid.UUID) error
	PurgeExpired(ctx context.Context) (int64, error)
}

// PostgresSandboxLeaseRepository implements SandboxLeaseRepository.
type PostgresSandboxLeaseRepository struct {
	db *sql.DB
}

// NewPostgresSandboxLeaseRepository instantiates a sandbox lease repository.
func NewPostgresSandboxLeaseRepository(db *sql.DB) *PostgresSandboxLeaseRepository {
	return &PostgresSandboxLeaseRepository{db: db}
}

// AcquireLease registers a newly provisioned sandbox lease.
func (r *PostgresSandboxLeaseRepository) AcquireLease(ctx context.Context, lease *SandboxLease) error {
	if lease.ID == uuid.Nil {
		lease.ID = uuid.New()
	}
	now := time.Now().UTC()
	lease.CreatedAt = now
	lease.UpdatedAt = now

	query := `
		INSERT INTO sandbox_leases (
			id, workspace_id, review_id, sandbox_provider, external_lease_id,
			ip_address, port, status, expires_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	_, err := r.db.ExecContext(ctx, query,
		lease.ID, lease.WorkspaceID, lease.ReviewID, lease.SandboxProvider,
		lease.ExternalLeaseID, lease.IPAddress, lease.Port, lease.Status,
		lease.ExpiresAt, lease.CreatedAt, lease.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed acquiring sandbox lease: %w", err)
	}
	return nil
}

// FindByReviewID retrieves the active lease attached to an ongoing review.
func (r *PostgresSandboxLeaseRepository) FindByReviewID(ctx context.Context, wsID, reviewID uuid.UUID) (*SandboxLease, error) {
	query := `
		SELECT id, workspace_id, review_id, sandbox_provider, external_lease_id,
		       ip_address, port, status, expires_at, released_at, created_at, updated_at
		FROM sandbox_leases
		WHERE workspace_id = $1 AND review_id = $2 AND status = 'ACTIVE' AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1
	`
	row := r.db.QueryRowContext(ctx, query, wsID, reviewID)
	var l SandboxLease
	err := row.Scan(
		&l.ID, &l.WorkspaceID, &l.ReviewID, &l.SandboxProvider, &l.ExternalLeaseID,
		&l.IPAddress, &l.Port, &l.Status, &l.ExpiresAt, &l.ReleasedAt, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching sandbox lease: %w", err)
	}
	return &l, nil
}

// ReleaseLease marks the sandbox lease as gracefully terminated.
func (r *PostgresSandboxLeaseRepository) ReleaseLease(ctx context.Context, wsID, id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE sandbox_leases
		SET status = 'RELEASED', released_at = $1, updated_at = $1
		WHERE workspace_id = $2 AND id = $3
	`
	_, err := r.db.ExecContext(ctx, query, now, wsID, id)
	return err
}

// PurgeExpired marks stale unreleased leases as expired.
func (r *PostgresSandboxLeaseRepository) PurgeExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	query := `
		UPDATE sandbox_leases
		SET status = 'EXPIRED', updated_at = $1
		WHERE status = 'ACTIVE' AND expires_at < $1
	`
	res, err := r.db.ExecContext(ctx, query, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
