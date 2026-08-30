package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Client wraps a thread-safe PostgreSQL connection pool with tenant RLS isolation support.
type Client struct {
	Pool *pgxpool.Pool
}

// NewClient initializes a connection pool to the Supabase / PostgreSQL database.
func NewClient(ctx context.Context, databaseURL string) (*Client, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database configuration: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 1
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 15 * time.Minute
	config.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}

	// Ping database with timeout (allow sufficient time for remote pooler TLS handshake)
	pingCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unreachable: %w", err)
	}

	return &Client{Pool: pool}, nil
}

// Close gracefully terminates all connections in the pool.
func (c *Client) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}

// ExecWithTenant runs a database operation within a transaction, strictly enforcing
// PostgreSQL Row-Level Security by setting the session-local tenant context.
func (c *Client) ExecWithTenant(ctx context.Context, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin tenant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Set session tenant ID for RLS policies (Master Rule 4.4)
	setTenantSQL := `SELECT set_config('app.current_tenant_id', $1, true)`
	if _, err := tx.Exec(ctx, setTenantSQL, tenantID.String()); err != nil {
		return fmt.Errorf("failed to set tenant RLS context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
