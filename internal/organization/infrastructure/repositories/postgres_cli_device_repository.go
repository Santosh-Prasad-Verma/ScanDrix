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
	clidevicedomain "github.com/scandrix/backend/internal/organization/domain/clidevice"
)

// PostgresCliDeviceRepository implements ICliDeviceRepository.
type PostgresCliDeviceRepository struct {
	pool   *pgxpool.Pool
	mu     sync.RWMutex
	memory map[string]*clidevicedomain.CliDeviceEntity // key: wsID:deviceID
}

// NewPostgresCliDeviceRepository creates a new repository.
func NewPostgresCliDeviceRepository(pool *pgxpool.Pool) *PostgresCliDeviceRepository {
	return &PostgresCliDeviceRepository{
		pool:   pool,
		memory: make(map[string]*clidevicedomain.CliDeviceEntity),
	}
}

func deviceKey(wsID uuid.UUID, deviceID string) string {
	return fmt.Sprintf("%s:%s", wsID.String(), deviceID)
}

// FindOne finds a registered device by workspace ID and device ID.
func (r *PostgresCliDeviceRepository) FindOne(ctx context.Context, wsID uuid.UUID, deviceID string) (*clidevicedomain.CliDeviceEntity, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if dev, ok := r.memory[deviceKey(wsID, deviceID)]; ok {
			return dev, nil
		}
		return nil, nil
	}

	query := `
		SELECT uuid, workspace_id, user_id, device_id, device_token_hash, user_agent, last_seen_at, created_at, updated_at
		FROM cli_devices
		WHERE workspace_id = $1 AND device_id = $2
		LIMIT 1
	`
	var dev clidevicedomain.CliDeviceEntity
	var userAgent *string
	var userID *uuid.UUID

	err := r.pool.QueryRow(ctx, query, wsID, deviceID).Scan(
		&dev.UUID, &dev.WorkspaceID, &userID, &dev.DeviceID, &dev.DeviceTokenHash,
		&userAgent, &dev.LastSeenAt, &dev.CreatedAt, &dev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query cli device: %w", err)
	}
	dev.UserID = userID
	if userAgent != nil {
		dev.UserAgent = *userAgent
	}
	return &dev, nil
}

// CountByWorkspaceID counts registered devices in a workspace.
func (r *PostgresCliDeviceRepository) CountByWorkspaceID(ctx context.Context, wsID uuid.UUID) (int, error) {
	if r.pool == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		count := 0
		for _, dev := range r.memory {
			if dev.WorkspaceID == wsID {
				count++
			}
		}
		return count, nil
	}

	query := `SELECT count(*) FROM cli_devices WHERE workspace_id = $1`
	var count int
	err := r.pool.QueryRow(ctx, query, wsID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count devices: %w", err)
	}
	return count, nil
}

// Create registers a new CLI device.
func (r *PostgresCliDeviceRepository) Create(ctx context.Context, entity *clidevicedomain.CliDeviceEntity) error {
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
	entity.LastSeenAt = now

	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.memory[deviceKey(entity.WorkspaceID, entity.DeviceID)] = entity
		return nil
	}

	query := `
		INSERT INTO cli_devices (uuid, workspace_id, user_id, device_id, device_token_hash, user_agent, last_seen_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (workspace_id, device_id) DO UPDATE SET
			user_id = COALESCE(EXCLUDED.user_id, cli_devices.user_id),
			device_token_hash = EXCLUDED.device_token_hash,
			user_agent = EXCLUDED.user_agent,
			last_seen_at = EXCLUDED.last_seen_at,
			updated_at = EXCLUDED.updated_at
	`
	_, err := r.pool.Exec(ctx, query,
		entity.UUID, entity.WorkspaceID, entity.UserID, entity.DeviceID, entity.DeviceTokenHash,
		entity.UserAgent, entity.LastSeenAt, entity.CreatedAt, entity.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert cli_device: %w", err)
	}
	return nil
}

// UpdateLastSeen updates device heartbeat.
func (r *PostgresCliDeviceRepository) UpdateLastSeen(ctx context.Context, id uuid.UUID, userAgent string) error {
	now := time.Now().UTC()
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, dev := range r.memory {
			if dev.UUID == id {
				dev.LastSeenAt = now
				dev.UpdatedAt = now
				if userAgent != "" {
					dev.UserAgent = userAgent
				}
				break
			}
		}
		return nil
	}

	query := `UPDATE cli_devices SET last_seen_at = $1, user_agent = CASE WHEN $2 != '' THEN $2 ELSE user_agent END, updated_at = $1 WHERE uuid = $3`
	_, err := r.pool.Exec(ctx, query, now, userAgent, id)
	return err
}

// UpdateTokenHash updates the hashed device token.
func (r *PostgresCliDeviceRepository) UpdateTokenHash(ctx context.Context, id uuid.UUID, tokenHash string, userAgent string) error {
	now := time.Now().UTC()
	if r.pool == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, dev := range r.memory {
			if dev.UUID == id {
				dev.DeviceTokenHash = tokenHash
				dev.LastSeenAt = now
				dev.UpdatedAt = now
				if userAgent != "" {
					dev.UserAgent = userAgent
				}
				break
			}
		}
		return nil
	}

	query := `UPDATE cli_devices SET device_token_hash = $1, last_seen_at = $2, user_agent = CASE WHEN $3 != '' THEN $3 ELSE user_agent END, updated_at = $2 WHERE uuid = $4`
	_, err := r.pool.Exec(ctx, query, tokenHash, now, userAgent, id)
	return err
}
