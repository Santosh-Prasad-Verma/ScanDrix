package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/core/domain"
)

// CliDeviceRepository defines database operations for registered CLI developer devices.
type CliDeviceRepository interface {
	FindByID(ctx context.Context, wsID, id uuid.UUID) (*domain.CliDevice, error)
	FindByDeviceIdentifier(ctx context.Context, wsID uuid.UUID, identifier string) (*domain.CliDevice, error)
	Create(ctx context.Context, device *domain.CliDevice) error
	UpdateLastSeen(ctx context.Context, wsID, id uuid.UUID, clientVersion string) error
	Revoke(ctx context.Context, wsID, id uuid.UUID) error
	ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*domain.CliDevice, error)
}

// PostgresCliDeviceRepository implements CliDeviceRepository using database/sql.
type PostgresCliDeviceRepository struct {
	db *sql.DB
}

// NewPostgresCliDeviceRepository instantiates a device repository.
func NewPostgresCliDeviceRepository(db *sql.DB) *PostgresCliDeviceRepository {
	return &PostgresCliDeviceRepository{db: db}
}

// FindByID retrieves a device by UUID.
func (r *PostgresCliDeviceRepository) FindByID(ctx context.Context, wsID, id uuid.UUID) (*domain.CliDevice, error) {
	query := `
		SELECT id, workspace_id, user_id, device_identifier, hostname, os, arch, client_version, last_seen_at, is_revoked, created_at, updated_at
		FROM cli_devices
		WHERE workspace_id = $1 AND id = $2
	`
	row := r.db.QueryRowContext(ctx, query, wsID, id)
	var d domain.CliDevice
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.UserID, &d.DeviceIdentifier, &d.Hostname, &d.OS, &d.Arch, &d.ClientVersion, &d.LastSeenAt, &d.IsRevoked, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching cli device %s: %w", id, err)
	}
	return &d, nil
}

// FindByDeviceIdentifier retrieves a device by hardware/client identifier.
func (r *PostgresCliDeviceRepository) FindByDeviceIdentifier(ctx context.Context, wsID uuid.UUID, identifier string) (*domain.CliDevice, error) {
	query := `
		SELECT id, workspace_id, user_id, device_identifier, hostname, os, arch, client_version, last_seen_at, is_revoked, created_at, updated_at
		FROM cli_devices
		WHERE workspace_id = $1 AND device_identifier = $2
	`
	row := r.db.QueryRowContext(ctx, query, wsID, identifier)
	var d domain.CliDevice
	err := row.Scan(&d.ID, &d.WorkspaceID, &d.UserID, &d.DeviceIdentifier, &d.Hostname, &d.OS, &d.Arch, &d.ClientVersion, &d.LastSeenAt, &d.IsRevoked, &d.CreatedAt, &d.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed fetching cli device identifier %s: %w", identifier, err)
	}
	return &d, nil
}

// Create registers a new developer CLI device.
func (r *PostgresCliDeviceRepository) Create(ctx context.Context, device *domain.CliDevice) error {
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now
	if device.LastSeenAt.IsZero() {
		device.LastSeenAt = now
	}

	query := `
		INSERT INTO cli_devices (id, workspace_id, user_id, device_identifier, hostname, os, arch, client_version, last_seen_at, is_revoked, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	_, err := r.db.ExecContext(ctx, query,
		device.ID, device.WorkspaceID, device.UserID, device.DeviceIdentifier,
		device.Hostname, device.OS, device.Arch, device.ClientVersion,
		device.LastSeenAt, device.IsRevoked, device.CreatedAt, device.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed creating cli device: %w", err)
	}
	return nil
}

// UpdateLastSeen updates timestamp and client version for telemetry.
func (r *PostgresCliDeviceRepository) UpdateLastSeen(ctx context.Context, wsID, id uuid.UUID, clientVersion string) error {
	now := time.Now().UTC()
	query := `
		UPDATE cli_devices
		SET last_seen_at = $1, client_version = $2, updated_at = $1
		WHERE workspace_id = $3 AND id = $4
	`
	res, err := r.db.ExecContext(ctx, query, now, clientVersion, wsID, id)
	if err != nil {
		return fmt.Errorf("failed updating last seen for cli device %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("cli device %s not found", id)
	}
	return nil
}

// Revoke revokes device access.
func (r *PostgresCliDeviceRepository) Revoke(ctx context.Context, wsID, id uuid.UUID) error {
	now := time.Now().UTC()
	query := `
		UPDATE cli_devices
		SET is_revoked = true, updated_at = $1
		WHERE workspace_id = $2 AND id = $3
	`
	_, err := r.db.ExecContext(ctx, query, now, wsID, id)
	if err != nil {
		return fmt.Errorf("failed revoking cli device %s: %w", id, err)
	}
	return nil
}

// ListByUser retrieves all devices registered by an engineer.
func (r *PostgresCliDeviceRepository) ListByUser(ctx context.Context, wsID, userID uuid.UUID) ([]*domain.CliDevice, error) {
	query := `
		SELECT id, workspace_id, user_id, device_identifier, hostname, os, arch, client_version, last_seen_at, is_revoked, created_at, updated_at
		FROM cli_devices
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY last_seen_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, wsID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed listing cli devices for user %s: %w", userID, err)
	}
	defer rows.Close()

	var devices []*domain.CliDevice
	for rows.Next() {
		var d domain.CliDevice
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.UserID, &d.DeviceIdentifier, &d.Hostname, &d.OS, &d.Arch, &d.ClientVersion, &d.LastSeenAt, &d.IsRevoked, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed scanning cli device: %w", err)
		}
		devices = append(devices, &d)
	}
	return devices, rows.Err()
}
