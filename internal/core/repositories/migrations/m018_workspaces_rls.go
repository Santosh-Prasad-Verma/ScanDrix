package migrations

import (
	"context"
)

// Migration018WorkspacesRls represents database migration 018_workspaces_rls.
type Migration018WorkspacesRls struct{}

// Version returns the unique migration version string.
func (m *Migration018WorkspacesRls) Version() string {
	return "018"
}

// Name returns the descriptive name of the migration.
func (m *Migration018WorkspacesRls) Name() string {
	return "018_workspaces_rls"
}

// Up applies the schema changes defined in 018_workspaces_rls.sql.
func (m *Migration018WorkspacesRls) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 018: Row-Level Security on Root Workspaces Table
-- Enforces strict multi-tenant boundaries on the root workspaces table
-- to prevent cross-tenant data leakage while permitting system workers to operate.

-- Enable and FORCE Row-Level Security on workspaces table
ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces FORCE ROW LEVEL SECURITY;

-- Create tenant isolation policy for workspaces
DROP POLICY IF EXISTS tenant_isolation_workspaces ON workspaces;
CREATE POLICY tenant_isolation_workspaces ON workspaces
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 018_workspaces_rls.sql.
func (m *Migration018WorkspacesRls) Down(ctx context.Context, exec SQLExecutor) error {
	query := `-- Rollback Workspaces RLS
SELECT 1;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
