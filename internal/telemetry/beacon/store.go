// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: store.go
// ═══════════════════════════════════════════════════════════════

package beacon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const TelemetryStateGlobalKey = "telemetry_state"

// TelemetryStateStore persists singleton instance telemetry coordinates.
type TelemetryStateStore interface {
	GetState(ctx context.Context) (*TelemetryStateValue, error)
	SetState(ctx context.Context, state *TelemetryStateValue) error
}

// PostgresTelemetryStateStore saves telemetry state in the global_parameters table.
type PostgresTelemetryStateStore struct {
	pool *pgxpool.Pool
}

// NewPostgresTelemetryStateStore builds a PostgreSQL-backed telemetry state repository.
func NewPostgresTelemetryStateStore(pool *pgxpool.Pool) *PostgresTelemetryStateStore {
	return &PostgresTelemetryStateStore{pool: pool}
}

// EnsureTable creates the global_parameters table if it does not yet exist.
func (s *PostgresTelemetryStateStore) EnsureTable(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("database connection pool is uninitialized")
	}
	query := `
		CREATE TABLE IF NOT EXISTS global_parameters (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			config_key VARCHAR(64) NOT NULL UNIQUE,
			config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_global_params_config_key ON global_parameters (config_key);
	`
	_, err := s.pool.Exec(ctx, query)
	return err
}

// GetState retrieves the active telemetry heartbeat state.
func (s *PostgresTelemetryStateStore) GetState(ctx context.Context) (*TelemetryStateValue, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("database connection pool is uninitialized")
	}

	query := `SELECT config_value FROM global_parameters WHERE config_key = $1;`
	var raw []byte
	err := s.pool.QueryRow(ctx, query, TelemetryStateGlobalKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var state TelemetryStateValue
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("corrupted telemetry state json: %w", err)
	}

	return &state, nil
}

// SetState persists updated telemetry coordinates with atomic UPSERT semantics.
func (s *PostgresTelemetryStateStore) SetState(ctx context.Context, state *TelemetryStateValue) error {
	if s.pool == nil {
		return fmt.Errorf("database connection pool is uninitialized")
	}
	if state == nil {
		return errors.New("state cannot be nil")
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed marshaling telemetry state: %w", err)
	}

	query := `
		INSERT INTO global_parameters (config_key, config_value, description, updated_at)
		VALUES ($1, $2::jsonb, 'Self-hosted heartbeat telemetry coordinator state', now())
		ON CONFLICT (config_key) DO UPDATE
		SET config_value = EXCLUDED.config_value,
		    updated_at = now();
	`
	_, err = s.pool.Exec(ctx, query, TelemetryStateGlobalKey, string(data))
	return err
}

// InMemoryTelemetryStateStore provides thread-safe in-memory state for testing and stateless runs.
type InMemoryTelemetryStateStore struct {
	mu    sync.RWMutex
	state *TelemetryStateValue
}

// NewInMemoryTelemetryStateStore constructs an in-memory telemetry state store.
func NewInMemoryTelemetryStateStore() *InMemoryTelemetryStateStore {
	return &InMemoryTelemetryStateStore{}
}

// GetState returns the current in-memory telemetry state.
func (m *InMemoryTelemetryStateStore) GetState(_ context.Context) (*TelemetryStateValue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state == nil {
		return nil, nil
	}
	// Return clone
	clone := *m.state
	return &clone, nil
}

// SetState stores the in-memory telemetry state.
func (m *InMemoryTelemetryStateStore) SetState(_ context.Context, state *TelemetryStateValue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state == nil {
		m.state = nil
		return nil
	}
	clone := *state
	m.state = &clone
	return nil
}
