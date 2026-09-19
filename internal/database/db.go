package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ═══════════════════════════════════════════════════════════════
// 1. POSTGRESQL CLIENT WRAPPER (Thread-safe pgx connection pool)
// ═══════════════════════════════════════════════════════════════

// Client wraps a thread-safe PostgreSQL connection pool with tenant RLS isolation support.
type Client struct {
	Pool *pgxpool.Pool
}

// ═══════════════════════════════════════════════════════════════
// 2. CONNECTION POOL INITIALIZATION & TUNING (PgBouncer protocol & limits)
// ═══════════════════════════════════════════════════════════════

// PoolOptions provides programmatic configuration of the PostgreSQL connection pool.
type PoolOptions struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	AcquireTimeout    time.Duration
}

// DefaultPoolOptions reads pool limits from the environment or falls back to enterprise defaults.
func DefaultPoolOptions() PoolOptions {
	var maxConns int32 = 25
	if val := os.Getenv("DATABASE_MAX_CONNS"); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 32); err == nil && parsed > 0 && parsed <= 1000 {
			maxConns = int32(parsed)
		}
	}
	var minConns int32 = 2
	if val := os.Getenv("DATABASE_MIN_CONNS"); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 32); err == nil && parsed >= 0 && parsed <= 1000 {
			minConns = int32(parsed)
		}
	}

	acquireTimeout := 20 * time.Second
	if val := os.Getenv("DATABASE_ACQUIRE_TIMEOUT"); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil && parsed > 0 {
			acquireTimeout = parsed
		}
	}

	return PoolOptions{
		MaxConns:          maxConns,
		MinConns:          minConns,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   15 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
		AcquireTimeout:    acquireTimeout,
	}
}

// NewClient initializes a connection pool using default environment-derived options.
func NewClient(ctx context.Context, databaseURL string) (*Client, error) {
	return NewClientWithOptions(ctx, databaseURL, DefaultPoolOptions())
}

// NewClientWithOptions initializes a connection pool with explicit programmatic pool options.
func NewClientWithOptions(ctx context.Context, databaseURL string, opts PoolOptions) (*Client, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database configuration: %w", err)
	}

	// Disable client-side prepared statement caching when connecting through PgBouncer / Supabase pooler
	// to prevent SQLSTATE 26000 (prepared statement does not exist).
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	if opts.MaxConns > 0 {
		config.MaxConns = opts.MaxConns
	} else {
		config.MaxConns = 25
	}
	if opts.MinConns >= 0 {
		config.MinConns = opts.MinConns
	} else {
		config.MinConns = 2
	}
	if opts.MaxConnLifetime > 0 {
		config.MaxConnLifetime = opts.MaxConnLifetime
	} else {
		config.MaxConnLifetime = 1 * time.Hour
	}
	if opts.MaxConnIdleTime > 0 {
		config.MaxConnIdleTime = opts.MaxConnIdleTime
	} else {
		config.MaxConnIdleTime = 15 * time.Minute
	}
	if opts.HealthCheckPeriod > 0 {
		config.HealthCheckPeriod = opts.HealthCheckPeriod
	} else {
		config.HealthCheckPeriod = 1 * time.Minute
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
	}

	pingTimeout := opts.AcquireTimeout
	if pingTimeout <= 0 {
		pingTimeout = 20 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database unreachable: %w", err)
	}

	return &Client{Pool: pool}, nil
}

// ═══════════════════════════════════════════════════════════════
// 3. POOL LIFECYCLE & HEALTH CHECKS (Graceful close & ping timeout)
// ═══════════════════════════════════════════════════════════════

// Close gracefully terminates all connections in the pool.
func (c *Client) Close() {
	if c.Pool != nil {
		c.Pool.Close()
	}
}

// Ping verifies connectivity to the underlying PostgreSQL pool.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Pool == nil {
		return fmt.Errorf("database client is nil or uninitialized")
	}
	return c.Pool.Ping(ctx)
}

// ═══════════════════════════════════════════════════════════════
// 4. MULTI-TENANT RLS ISOLATION & SYSTEM TRANSACTIONS (Row-level security enforcement)
// ═══════════════════════════════════════════════════════════════

// ExecWithTenant runs a database operation within a transaction, strictly enforcing
// PostgreSQL Row-Level Security by setting the session-local tenant context.
func (c *Client) ExecWithTenant(ctx context.Context, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error {
	if c == nil || c.Pool == nil {
		return fmt.Errorf("database pool unavailable")
	}
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin tenant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Set session tenant ID for RLS policies (Master Rule 4.4)
	setTenantSQL := `SELECT set_config('app.current_tenant_id', $1, true), set_config('app.current_workspace_id', $1, true)`
	if _, err := tx.Exec(ctx, setTenantSQL, tenantID.String()); err != nil {
		return fmt.Errorf("failed to set tenant RLS context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ExecAsSystem runs a database operation within a transaction as a privileged system background worker,
// setting app.is_system_worker to true so background jobs (outbox relay, aggregations) can operate safely across tenants.
func (c *Client) ExecAsSystem(ctx context.Context, fn func(tx pgx.Tx) error) error {
	if c == nil || c.Pool == nil {
		return fmt.Errorf("database pool unavailable")
	}
	tx, err := c.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin system transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	setSystemSQL := `SELECT set_config('app.is_system_worker', 'true', true)`
	if _, err := tx.Exec(ctx, setSystemSQL); err != nil {
		return fmt.Errorf("failed to set system worker context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
