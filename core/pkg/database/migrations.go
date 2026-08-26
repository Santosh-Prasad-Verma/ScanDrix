package database

import (
	"context"
	"fmt"
	"time"

	"github.com/codehound/codehound/core/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	CurrentSchemaVersion = 1
	UpMigrationFile      = "000001_init_schema.up.sql"
	DownMigrationFile    = "000001_init_schema.down.sql"
)

// EnsureSchemaMigrationsTable initializes the tracking table if not present.
func EnsureSchemaMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		);
	`
	_, err := pool.Exec(ctx, query)
	return err
}

// GetCurrentVersion returns the latest applied schema version (or 0 if none).
func GetCurrentVersion(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if err := EnsureSchemaMigrationsTable(ctx, pool); err != nil {
		return 0, err
	}

	var version int
	query := `SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`
	err := pool.QueryRow(ctx, query).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("failed to query schema_migrations: %w", err)
	}
	return version, nil
}

// MigrateUp executes the up migration script.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	current, err := GetCurrentVersion(ctx, pool)
	if err != nil {
		return err
	}

	if current >= CurrentSchemaVersion {
		return fmt.Errorf("database is already up to date at version %d", current)
	}

	sqlBytes, err := migrations.FS.ReadFile(UpMigrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", UpMigrationFile, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Execute DDL
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("migration up failed: %w", err)
	}

	// Record version
	recordQuery := `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2);`
	if _, err := tx.Exec(ctx, recordQuery, CurrentSchemaVersion, time.Now()); err != nil {
		return fmt.Errorf("failed to record schema version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	return nil
}

// MigrateDown executes the down migration script.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	current, err := GetCurrentVersion(ctx, pool)
	if err != nil {
		return err
	}

	if current == 0 {
		return fmt.Errorf("database has no applied migrations to roll back")
	}

	sqlBytes, err := migrations.FS.ReadFile(DownMigrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", DownMigrationFile, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin rollback transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Execute rollback DDL
	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("migration down failed: %w", err)
	}

	// Remove version record
	removeQuery := `DELETE FROM schema_migrations WHERE version = $1;`
	if _, err := tx.Exec(ctx, removeQuery, CurrentSchemaVersion); err != nil {
		return fmt.Errorf("failed to remove schema version record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit rollback transaction: %w", err)
	}

	return nil
}
