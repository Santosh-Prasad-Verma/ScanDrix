package migrations

import (
	"context"
)

// Migration012SystemWorkerOutboxRls represents database migration 012_system_worker_outbox_rls.
type Migration012SystemWorkerOutboxRls struct{}

// Version returns the unique migration version string.
func (m *Migration012SystemWorkerOutboxRls) Version() string {
	return "012"
}

// Name returns the descriptive name of the migration.
func (m *Migration012SystemWorkerOutboxRls) Name() string {
	return "012_system_worker_outbox_rls"
}

// Up applies the schema changes defined in 012_system_worker_outbox_rls.sql.
func (m *Migration012SystemWorkerOutboxRls) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 012: System Worker RLS Policy for Outbox Events
-- Allows background system worker processes to query and dispatch outbox events across tenants
-- when app.is_system_worker = 'true' is set in the session/transaction.

DROP POLICY IF EXISTS tenant_isolation_outbox ON outbox_events;
CREATE POLICY tenant_isolation_outbox ON outbox_events
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 012_system_worker_outbox_rls.sql.
func (m *Migration012SystemWorkerOutboxRls) Down(ctx context.Context, exec SQLExecutor) error {
	query := `-- Rollback system worker outbox RLS
SELECT 1;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
