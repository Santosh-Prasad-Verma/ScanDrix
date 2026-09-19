package lease

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/sandbox/contracts"
)

// ISandboxLeaseRepository defines atomic storage and state management operations for sandbox coordination leases.
type ISandboxLeaseRepository interface {
	UpsertAcquire(ctx context.Context, prKey string, leaseTTL time.Duration, consumer string) (*SandboxLease, error)
	DecrementLease(ctx context.Context, prKey string) (*SandboxLease, error)
	UpdateReady(ctx context.Context, prKey, sandboxID string) error
	MarkInvalidated(ctx context.Context, prKey string) error
	FindByPrKey(ctx context.Context, prKey string) (*SandboxLease, error)
	FindExpired(ctx context.Context, now time.Time) ([]*SandboxLease, error)
	Delete(ctx context.Context, prKey string) error
	SetKillAt(ctx context.Context, prKey string, killAt time.Time) error
	ClearKillAt(ctx context.Context, prKey string) error
	FindReadyToKill(ctx context.Context, now time.Time) ([]*SandboxLease, error)
	ClaimCleanup(ctx context.Context, prKey, expectedSandboxID string, requireLeaseCountZero bool) (*SandboxLease, error)
	CompleteCleanup(ctx context.Context, prKey, expectedSandboxID string) (bool, error)
	FailCleanup(ctx context.Context, prKey, expectedSandboxID, errMsg string) (bool, error)
	ResetStaleCleanup(ctx context.Context, prKey string, staleThreshold time.Time) error
}

// ═══════════════════════════════════════════════════════════════
// 1. POSTGRESQL LEASE REPOSITORY IMPLEMENTATION
// ═══════════════════════════════════════════════════════════════

type PgSandboxLeaseRepository struct {
	pool *pgxpool.Pool
}

func NewPgSandboxLeaseRepository(pool *pgxpool.Pool) *PgSandboxLeaseRepository {
	return &PgSandboxLeaseRepository{pool: pool}
}

func (r *PgSandboxLeaseRepository) UpsertAcquire(ctx context.Context, prKey string, leaseTTL time.Duration, consumer string) (*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	dec, err := contracts.DecomposePrKey(prKey)
	if err != nil {
		return nil, err
	}

	var orgUUID *uuid.UUID
	if parsed, err := uuid.Parse(dec.OrganizationID); err == nil {
		orgUUID = &parsed
	}

	now := time.Now().UTC()
	expiresAt := now.Add(leaseTTL)

	query := `
		INSERT INTO sandbox_leases (
			pr_key, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, 'CREATING', 1, $6, $7)
		ON CONFLICT (pr_key) DO UPDATE SET
			lease_count = sandbox_leases.lease_count + 1,
			consumer = CASE WHEN EXCLUDED.consumer IS NOT NULL AND EXCLUDED.consumer != '' THEN EXCLUDED.consumer ELSE sandbox_leases.consumer END
		RETURNING pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at;
	`

	var lease SandboxLease
	var orgID *uuid.UUID
	var cleanStatus *string

	err = r.pool.QueryRow(ctx, query,
		prKey, orgUUID, dec.RepositoryID, dec.PRNumber, consumer, now, expiresAt,
	).Scan(
		&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
		&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
		&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
		&lease.CleanupError, &lease.CleanupStartedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert acquire sandbox lease: %w", err)
	}

	lease.OrganizationID = orgID
	if cleanStatus != nil {
		lease.CleanupStatus = CleanupStatus(*cleanStatus)
	}

	return &lease, nil
}

func (r *PgSandboxLeaseRepository) DecrementLease(ctx context.Context, prKey string) (*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET lease_count = lease_count - 1
		WHERE pr_key = $1
		RETURNING pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at;
	`

	var lease SandboxLease
	var orgID *uuid.UUID
	var cleanStatus *string

	err := r.pool.QueryRow(ctx, query, prKey).Scan(
		&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
		&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
		&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
		&lease.CleanupError, &lease.CleanupStartedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decrement lease: %w", err)
	}

	lease.OrganizationID = orgID
	if cleanStatus != nil {
		lease.CleanupStatus = CleanupStatus(*cleanStatus)
	}
	return &lease, nil
}

func (r *PgSandboxLeaseRepository) UpdateReady(ctx context.Context, prKey, sandboxID string) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET state = 'READY', sandbox_id = $2
		WHERE pr_key = $1 AND state = 'CREATING';
	`
	_, err := r.pool.Exec(ctx, query, prKey, sandboxID)
	return err
}

func (r *PgSandboxLeaseRepository) MarkInvalidated(ctx context.Context, prKey string) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET state = 'INVALIDATED'
		WHERE pr_key = $1 AND state IN ('CREATING', 'READY');
	`
	_, err := r.pool.Exec(ctx, query, prKey)
	return err
}

func (r *PgSandboxLeaseRepository) FindByPrKey(ctx context.Context, prKey string) (*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at
		FROM sandbox_leases
		WHERE pr_key = $1;
	`

	var lease SandboxLease
	var orgID *uuid.UUID
	var cleanStatus *string

	err := r.pool.QueryRow(ctx, query, prKey).Scan(
		&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
		&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
		&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
		&lease.CleanupError, &lease.CleanupStartedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	lease.OrganizationID = orgID
	if cleanStatus != nil {
		lease.CleanupStatus = CleanupStatus(*cleanStatus)
	}
	return &lease, nil
}

func (r *PgSandboxLeaseRepository) FindExpired(ctx context.Context, now time.Time) ([]*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at
		FROM sandbox_leases
		WHERE expires_at < $1;
	`

	rows, err := r.pool.Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*SandboxLease
	for rows.Next() {
		var lease SandboxLease
		var orgID *uuid.UUID
		var cleanStatus *string
		if err := rows.Scan(
			&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
			&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
			&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
			&lease.CleanupError, &lease.CleanupStartedAt,
		); err != nil {
			return nil, err
		}
		lease.OrganizationID = orgID
		if cleanStatus != nil {
			lease.CleanupStatus = CleanupStatus(*cleanStatus)
		}
		result = append(result, &lease)
	}

	return result, rows.Err()
}

func (r *PgSandboxLeaseRepository) Delete(ctx context.Context, prKey string) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `DELETE FROM sandbox_leases WHERE pr_key = $1;`
	_, err := r.pool.Exec(ctx, query, prKey)
	return err
}

func (r *PgSandboxLeaseRepository) SetKillAt(ctx context.Context, prKey string, killAt time.Time) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET kill_at = $2
		WHERE pr_key = $1 AND sandbox_id IS NOT NULL AND sandbox_id != '';
	`
	_, err := r.pool.Exec(ctx, query, prKey, killAt)
	return err
}

func (r *PgSandboxLeaseRepository) ClearKillAt(ctx context.Context, prKey string) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `UPDATE sandbox_leases SET kill_at = NULL WHERE pr_key = $1;`
	_, err := r.pool.Exec(ctx, query, prKey)
	return err
}

func (r *PgSandboxLeaseRepository) FindReadyToKill(ctx context.Context, now time.Time) ([]*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		SELECT pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at
		FROM sandbox_leases
		WHERE kill_at <= $1 AND sandbox_id IS NOT NULL AND sandbox_id != '';
	`

	rows, err := r.pool.Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*SandboxLease
	for rows.Next() {
		var lease SandboxLease
		var orgID *uuid.UUID
		var cleanStatus *string
		if err := rows.Scan(
			&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
			&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
			&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
			&lease.CleanupError, &lease.CleanupStartedAt,
		); err != nil {
			return nil, err
		}
		lease.OrganizationID = orgID
		if cleanStatus != nil {
			lease.CleanupStatus = CleanupStatus(*cleanStatus)
		}
		result = append(result, &lease)
	}

	return result, rows.Err()
}

func (r *PgSandboxLeaseRepository) ClaimCleanup(ctx context.Context, prKey, expectedSandboxID string, requireLeaseCountZero bool) (*SandboxLease, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET cleanup_status = 'in_progress',
			cleanup_retry_at = NULL,
			cleanup_started_at = $3,
			cleanup_attempts = cleanup_attempts + 1
		WHERE pr_key = $1
		  AND sandbox_id = $2
		  AND (cleanup_status IS NULL OR cleanup_status NOT IN ('in_progress', 'completed'))
		  AND ($4 = false OR lease_count <= 0)
		RETURNING pr_key, sandbox_id, organization_id, repository_id, pr_number, consumer, state, lease_count, created_at, expires_at, kill_at, cleanup_status, cleanup_attempts, cleanup_retry_at, cleanup_error, cleanup_started_at;
	`

	var lease SandboxLease
	var orgID *uuid.UUID
	var cleanStatus *string

	err := r.pool.QueryRow(ctx, query, prKey, expectedSandboxID, time.Now().UTC(), requireLeaseCountZero).Scan(
		&lease.PrKey, &lease.SandboxID, &orgID, &lease.RepositoryID, &lease.PRNumber,
		&lease.Consumer, &lease.State, &lease.LeaseCount, &lease.CreatedAt, &lease.ExpiresAt,
		&lease.KillAt, &cleanStatus, &lease.CleanupAttempts, &lease.CleanupRetryAt,
		&lease.CleanupError, &lease.CleanupStartedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	lease.OrganizationID = orgID
	if cleanStatus != nil {
		lease.CleanupStatus = CleanupStatus(*cleanStatus)
	}
	return &lease, nil
}

func (r *PgSandboxLeaseRepository) CompleteCleanup(ctx context.Context, prKey, expectedSandboxID string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, errors.New("database pool uninitialized")
	}

	query := `
		DELETE FROM sandbox_leases
		WHERE pr_key = $1 AND sandbox_id = $2 AND cleanup_status = 'in_progress';
	`
	cmd, err := r.pool.Exec(ctx, query, prKey, expectedSandboxID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() == 1, nil
}

func (r *PgSandboxLeaseRepository) FailCleanup(ctx context.Context, prKey, expectedSandboxID, errMsg string) (bool, error) {
	if r == nil || r.pool == nil {
		return false, errors.New("database pool uninitialized")
	}

	retryAt := time.Now().UTC().Add(60 * time.Second)
	truncatedErr := errMsg
	if len(truncatedErr) > 500 {
		truncatedErr = truncatedErr[:500]
	}

	query := `
		UPDATE sandbox_leases
		SET cleanup_status = 'failed',
			cleanup_retry_at = $3,
			cleanup_error = $4
		WHERE pr_key = $1 AND sandbox_id = $2 AND cleanup_status = 'in_progress';
	`
	cmd, err := r.pool.Exec(ctx, query, prKey, expectedSandboxID, retryAt, truncatedErr)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() == 1, nil
}

func (r *PgSandboxLeaseRepository) ResetStaleCleanup(ctx context.Context, prKey string, staleThreshold time.Time) error {
	if r == nil || r.pool == nil {
		return errors.New("database pool uninitialized")
	}

	query := `
		UPDATE sandbox_leases
		SET cleanup_status = 'failed',
			cleanup_error = 'Worker crash recovery — stale in_progress reset'
		WHERE pr_key = $1
		  AND cleanup_status = 'in_progress'
		  AND cleanup_started_at < $2;
	`
	_, err := r.pool.Exec(ctx, query, prKey, staleThreshold)
	return err
}

// ═══════════════════════════════════════════════════════════════
// 2. IN-MEMORY THREAD-SAFE LEASE REPOSITORY (For tests & standalone)
// ═══════════════════════════════════════════════════════════════

type MemorySandboxLeaseRepository struct {
	mu     sync.RWMutex
	leases map[string]*SandboxLease
}

func NewMemorySandboxLeaseRepository() *MemorySandboxLeaseRepository {
	return &MemorySandboxLeaseRepository{
		leases: make(map[string]*SandboxLease),
	}
}

func (m *MemorySandboxLeaseRepository) UpsertAcquire(ctx context.Context, prKey string, leaseTTL time.Duration, consumer string) (*SandboxLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dec, err := contracts.DecomposePrKey(prKey)
	if err != nil {
		return nil, err
	}

	var orgUUID *uuid.UUID
	if parsed, err := uuid.Parse(dec.OrganizationID); err == nil {
		orgUUID = &parsed
	}

	now := time.Now().UTC()
	expiresAt := now.Add(leaseTTL)

	existing, ok := m.leases[prKey]
	if !ok {
		created := &SandboxLease{
			PrKey:          prKey,
			OrganizationID: orgUUID,
			RepositoryID:   dec.RepositoryID,
			PRNumber:       dec.PRNumber,
			Consumer:       consumer,
			State:          StateCreating,
			LeaseCount:     1,
			CreatedAt:      now,
			ExpiresAt:      expiresAt,
		}
		m.leases[prKey] = created
		copy := *created
		return &copy, nil
	}

	// Update path
	existing.LeaseCount++
	if consumer != "" {
		existing.Consumer = consumer
	}
	copy := *existing
	return &copy, nil
}

func (m *MemorySandboxLeaseRepository) DecrementLease(ctx context.Context, prKey string) (*SandboxLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if !ok {
		return nil, nil
	}

	lease.LeaseCount--
	copy := *lease
	return &copy, nil
}

func (m *MemorySandboxLeaseRepository) UpdateReady(ctx context.Context, prKey, sandboxID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if ok && lease.State == StateCreating {
		lease.State = StateReady
		lease.SandboxID = sandboxID
	}
	return nil
}

func (m *MemorySandboxLeaseRepository) MarkInvalidated(ctx context.Context, prKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if ok && (lease.State == StateCreating || lease.State == StateReady) {
		lease.State = StateInvalidated
	}
	return nil
}

func (m *MemorySandboxLeaseRepository) FindByPrKey(ctx context.Context, prKey string) (*SandboxLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lease, ok := m.leases[prKey]
	if !ok {
		return nil, nil
	}
	copy := *lease
	return &copy, nil
}

func (m *MemorySandboxLeaseRepository) FindExpired(ctx context.Context, now time.Time) ([]*SandboxLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var expired []*SandboxLease
	for _, l := range m.leases {
		if l.ExpiresAt.Before(now) {
			copy := *l
			expired = append(expired, &copy)
		}
	}
	return expired, nil
}

func (m *MemorySandboxLeaseRepository) Delete(ctx context.Context, prKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.leases, prKey)
	return nil
}

func (m *MemorySandboxLeaseRepository) SetKillAt(ctx context.Context, prKey string, killAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if ok && lease.SandboxID != "" {
		t := killAt
		lease.KillAt = &t
	}
	return nil
}

func (m *MemorySandboxLeaseRepository) ClearKillAt(ctx context.Context, prKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if ok {
		lease.KillAt = nil
	}
	return nil
}

func (m *MemorySandboxLeaseRepository) FindReadyToKill(ctx context.Context, now time.Time) ([]*SandboxLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ready []*SandboxLease
	for _, l := range m.leases {
		if l.KillAt != nil && !l.KillAt.After(now) && l.SandboxID != "" {
			copy := *l
			ready = append(ready, &copy)
		}
	}
	return ready, nil
}

func (m *MemorySandboxLeaseRepository) ClaimCleanup(ctx context.Context, prKey, expectedSandboxID string, requireLeaseCountZero bool) (*SandboxLease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if !ok || lease.SandboxID != expectedSandboxID {
		return nil, nil
	}

	if lease.CleanupStatus == CleanupInProgress || lease.CleanupStatus == CleanupCompleted {
		return nil, nil
	}

	if requireLeaseCountZero && lease.LeaseCount > 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	lease.CleanupStatus = CleanupInProgress
	lease.CleanupRetryAt = nil
	lease.CleanupStartedAt = &now
	lease.CleanupAttempts++

	copy := *lease
	return &copy, nil
}

func (m *MemorySandboxLeaseRepository) CompleteCleanup(ctx context.Context, prKey, expectedSandboxID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if !ok || lease.SandboxID != expectedSandboxID || lease.CleanupStatus != CleanupInProgress {
		return false, nil
	}

	delete(m.leases, prKey)
	return true, nil
}

func (m *MemorySandboxLeaseRepository) FailCleanup(ctx context.Context, prKey, expectedSandboxID, errMsg string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if !ok || lease.SandboxID != expectedSandboxID || lease.CleanupStatus != CleanupInProgress {
		return false, nil
	}

	retry := time.Now().UTC().Add(60 * time.Second)
	lease.CleanupStatus = CleanupFailed
	lease.CleanupRetryAt = &retry
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}
	lease.CleanupError = errMsg
	return true, nil
}

func (m *MemorySandboxLeaseRepository) ResetStaleCleanup(ctx context.Context, prKey string, staleThreshold time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lease, ok := m.leases[prKey]
	if !ok {
		return nil
	}

	if lease.CleanupStatus == CleanupInProgress && lease.CleanupStartedAt != nil && lease.CleanupStartedAt.Before(staleThreshold) {
		lease.CleanupStatus = CleanupFailed
		lease.CleanupError = "Worker crash recovery — stale in_progress reset"
	}
	return nil
}
