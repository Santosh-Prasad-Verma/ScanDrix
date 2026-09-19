// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Shared Database Module
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SharedPostgresOptions defines configuration parameters for PostgreSQL connection pooling.
type SharedPostgresOptions struct {
	URL             string        `json:"url"`
	PoolSize        int           `json:"pool_size"`
	MinConnections  int           `json:"min_connections"`
	MaxConnLifetime time.Duration `json:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time"`
	HealthTimeout   time.Duration `json:"health_timeout"`
}

// DefaultSharedPostgresOptions initializes sensible defaults from environment variables.
func DefaultSharedPostgresOptions() SharedPostgresOptions {
	poolSize := 20
	if val := os.Getenv("DATABASE_POOL_SIZE"); val != "" {
		if p, err := strconv.Atoi(val); err == nil && p > 0 {
			poolSize = p
		}
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		host := os.Getenv("POSTGRES_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("POSTGRES_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("POSTGRES_USER")
		if user == "" {
			user = "postgres"
		}
		pass := os.Getenv("POSTGRES_PASSWORD")
		if pass == "" {
			pass = "postgres"
		}
		db := os.Getenv("POSTGRES_DB")
		if db == "" {
			db = "scandrix"
		}
		url = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, db)
	}

	return SharedPostgresOptions{
		URL:             url,
		PoolSize:        poolSize,
		MinConnections:  5,
		MaxConnLifetime: 1 * time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
		HealthTimeout:   3 * time.Second,
	}
}

// SharedPostgresPool manages the PostgreSQL pgxpool connection pool.
type SharedPostgresPool struct {
	pool    *pgxpool.Pool
	options SharedPostgresOptions
	logger  *slog.Logger
}

// NewSharedPostgresPool constructs a configured pool instance.
func NewSharedPostgresPool(opts SharedPostgresOptions, logger *slog.Logger) *SharedPostgresPool {
	if logger == nil {
		logger = slog.Default()
	}
	return &SharedPostgresPool{
		options: opts,
		logger:  logger.With("component", "shared_postgres"),
	}
}

// Connect establishes the connection pool.
func (p *SharedPostgresPool) Connect(ctx context.Context) error {
	config, err := pgxpool.ParseConfig(p.options.URL)
	if err != nil {
		return fmt.Errorf("failed parsing postgres connection string: %w", err)
	}

	if p.options.PoolSize > 0 {
		config.MaxConns = int32(p.options.PoolSize)
	}
	if p.options.MinConnections > 0 {
		config.MinConns = int32(p.options.MinConnections)
	}
	if p.options.MaxConnLifetime > 0 {
		config.MaxConnLifetime = p.options.MaxConnLifetime
	}
	if p.options.MaxConnIdleTime > 0 {
		config.MaxConnIdleTime = p.options.MaxConnIdleTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed creating postgres pool: %w", err)
	}

	p.pool = pool
	p.logger.Info("PostgreSQL connection pool initialized",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
	)
	return nil
}

// Pool returns the underlying pgxpool.Pool.
func (p *SharedPostgresPool) Pool() *pgxpool.Pool {
	return p.pool
}

// Ping verifies database connectivity.
func (p *SharedPostgresPool) Ping(ctx context.Context) error {
	if p.pool == nil {
		return fmt.Errorf("postgres pool not connected")
	}
	timeout := p.options.HealthTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return p.pool.Ping(pingCtx)
}

// Close gracefully terminates all connections in the pool.
func (p *SharedPostgresPool) Close() {
	if p.pool != nil {
		p.pool.Close()
		p.logger.Info("PostgreSQL connection pool closed")
	}
}
