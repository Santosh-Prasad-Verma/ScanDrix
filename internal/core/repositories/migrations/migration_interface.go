package migrations

import (
	"context"
	"database/sql"
	"time"
)

// SQLExecutor captures common query execution methods between *sql.DB and *sql.Tx.
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Migration mirrors TypeORM MigrationInterface in Go.
type Migration interface {
	Version() string
	Name() string
	Up(ctx context.Context, exec SQLExecutor) error
	Down(ctx context.Context, exec SQLExecutor) error
}

// MigrationRecord mirrors schema_migrations row.
type MigrationRecord struct {
	Version   string    `json:"version" db:"version"`
	AppliedAt time.Time `json:"applied_at" db:"applied_at"`
}

// MigrationStatus reports applied/pending state of a registered migration.
type MigrationStatus struct {
	Version   string     `json:"version"`
	Name      string     `json:"name"`
	Applied   bool       `json:"applied"`
	AppliedAt *time.Time `json:"applied_at,omitempty"`
}
