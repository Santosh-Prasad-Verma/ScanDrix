package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MigrationRunner orchestrates type-safe compiled database migrations.
type MigrationRunner struct {
	db         *sql.DB
	migrations []Migration
	mu         sync.RWMutex
}

// NewMigrationRunner instantiates a migration runner with registered migrations.
func NewMigrationRunner(db *sql.DB) *MigrationRunner {
	return &MigrationRunner{
		db:         db,
		migrations: make([]Migration, 0),
	}
}

// Register adds a migration to the runner.
func (r *MigrationRunner) Register(m Migration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.migrations = append(r.migrations, m)
	sort.Slice(r.migrations, func(i, j int) bool {
		return r.migrations[i].Version() < r.migrations[j].Version()
	})
}

// EnsureSchemaMigrations ensures the tracking table exists.
func (r *MigrationRunner) EnsureSchemaMigrations(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`
	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed ensuring schema_migrations table: %w", err)
	}
	return nil
}

// GetApplied returns a set of already applied migration versions.
func (r *MigrationRunner) GetApplied(ctx context.Context) (map[string]time.Time, error) {
	if err := r.EnsureSchemaMigrations(ctx); err != nil {
		return nil, err
	}

	query := `SELECT version, applied_at FROM schema_migrations`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed querying schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var v string
		var t time.Time
		if err := rows.Scan(&v, &t); err != nil {
			return nil, err
		}
		applied[v] = t
	}
	return applied, rows.Err()
}

// Up executes all unapplied registered migrations in forward chronological order.
func (r *MigrationRunner) Up(ctx context.Context) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	applied, err := r.GetApplied(ctx)
	if err != nil {
		return nil, err
	}

	var executed []string
	for _, m := range r.migrations {
		if _, exists := applied[m.Version()]; exists {
			continue
		}

		tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return executed, fmt.Errorf("failed starting transaction for migration %s: %w", m.Version(), err)
		}

		if err := m.Up(ctx, tx); err != nil {
			_ = tx.Rollback()
			return executed, fmt.Errorf("migration %s (%s) failed during UP: %w", m.Version(), m.Name(), err)
		}

		recordQuery := `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)`
		if _, err := tx.ExecContext(ctx, recordQuery, m.Version(), time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return executed, fmt.Errorf("failed recording migration %s: %w", m.Version(), err)
		}

		if err := tx.Commit(); err != nil {
			return executed, fmt.Errorf("failed committing migration %s: %w", m.Version(), err)
		}

		executed = append(executed, m.Version())
	}

	return executed, nil
}

// Down rolls back a specified number of migrations in reverse chronological order.
func (r *MigrationRunner) Down(ctx context.Context, steps int) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if steps <= 0 {
		return nil, nil
	}

	applied, err := r.GetApplied(ctx)
	if err != nil {
		return nil, err
	}

	// Filter down to applied migrations in reverse order
	var toRollback []Migration
	for i := len(r.migrations) - 1; i >= 0; i-- {
		m := r.migrations[i]
		if _, exists := applied[m.Version()]; exists {
			toRollback = append(toRollback, m)
			if len(toRollback) == steps {
				break
			}
		}
	}

	var rolledBack []string
	for _, m := range toRollback {
		tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			return rolledBack, fmt.Errorf("failed starting rollback transaction for %s: %w", m.Version(), err)
		}

		if err := m.Down(ctx, tx); err != nil {
			_ = tx.Rollback()
			return rolledBack, fmt.Errorf("migration %s (%s) failed during DOWN: %w", m.Version(), m.Name(), err)
		}

		deleteQuery := `DELETE FROM schema_migrations WHERE version = $1`
		if _, err := tx.ExecContext(ctx, deleteQuery, m.Version()); err != nil {
			_ = tx.Rollback()
			return rolledBack, fmt.Errorf("failed removing migration record %s: %w", m.Version(), err)
		}

		if err := tx.Commit(); err != nil {
			return rolledBack, fmt.Errorf("failed committing rollback %s: %w", m.Version(), err)
		}

		rolledBack = append(rolledBack, m.Version())
	}

	return rolledBack, nil
}

// Status returns current migration state for all registered migrations.
func (r *MigrationRunner) Status(ctx context.Context) ([]MigrationStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	applied, err := r.GetApplied(ctx)
	if err != nil {
		return nil, err
	}

	statuses := make([]MigrationStatus, 0, len(r.migrations))
	for _, m := range r.migrations {
		appliedAt, isApplied := applied[m.Version()]
		var appliedAtPtr *time.Time
		if isApplied {
			appliedAtPtr = &appliedAt
		}
		statuses = append(statuses, MigrationStatus{
			Version:   m.Version(),
			Name:      m.Name(),
			Applied:   isApplied,
			AppliedAt: appliedAtPtr,
		})
	}
	return statuses, nil
}
