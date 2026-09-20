package migrations

import (
	"context"
)

// Migration013InboxEvents represents database migration 013_inbox_events.
type Migration013InboxEvents struct{}

// Version returns the unique migration version string.
func (m *Migration013InboxEvents) Version() string {
	return "013"
}

// Name returns the descriptive name of the migration.
func (m *Migration013InboxEvents) Name() string {
	return "013_inbox_events"
}

// Up applies the schema changes defined in 013_inbox_events.sql.
func (m *Migration013InboxEvents) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 013: Inbox Deduplicator for Distributed Worker Idempotency
-- Enforces cluster-wide exactly-once message consumption across worker replicas

CREATE TABLE IF NOT EXISTS inbox_records (
    id VARCHAR(255) PRIMARY KEY, -- message_id:consumer_id
    message_id VARCHAR(255) NOT NULL,
    consumer_id VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PROCESSING',
    attempt_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inbox_message_id ON inbox_records(message_id);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 013_inbox_events.sql.
func (m *Migration013InboxEvents) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS inbox_records CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
