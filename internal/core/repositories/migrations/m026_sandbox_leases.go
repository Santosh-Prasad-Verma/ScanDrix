package migrations

import (
	"context"
)

// Migration026SandboxLeases represents database migration 026_sandbox_leases.
type Migration026SandboxLeases struct{}

// Version returns the unique migration version string.
func (m *Migration026SandboxLeases) Version() string {
	return "026"
}

// Name returns the descriptive name of the migration.
func (m *Migration026SandboxLeases) Name() string {
	return "026_sandbox_leases"
}

// Up applies the schema changes defined in 026_sandbox_leases.sql.
func (m *Migration026SandboxLeases) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 026: Sandbox Leases for MicroVM and Ephemeral Worktree Lifecycle Coordination
-- Persists sandbox_leases into hardened PostgreSQL with tenant RLS isolation

CREATE TABLE IF NOT EXISTS sandbox_leases (
    pr_key VARCHAR(255) PRIMARY KEY,
    sandbox_id TEXT,
    organization_id UUID,
    repository_id TEXT,
    pr_number TEXT,
    consumer TEXT,
    state VARCHAR(32) NOT NULL DEFAULT 'CREATING' CHECK (state IN ('CREATING', 'READY', 'PAUSED', 'INVALIDATED')),
    lease_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    kill_at TIMESTAMPTZ,
    cleanup_status VARCHAR(32) CHECK (cleanup_status IS NULL OR cleanup_status IN ('pending', 'in_progress', 'completed', 'failed')),
    cleanup_attempts INT NOT NULL DEFAULT 0,
    cleanup_retry_at TIMESTAMPTZ,
    cleanup_error TEXT,
    cleanup_started_at TIMESTAMPTZ
);

-- Range scan for reaper: find expired leases past TTL
CREATE INDEX IF NOT EXISTS idx_sandbox_leases_expires_at ON sandbox_leases(expires_at);

-- Invalidate / lookup by sandbox ID (sparse)
CREATE INDEX IF NOT EXISTS idx_sandbox_leases_sandbox_id ON sandbox_leases(sandbox_id) WHERE sandbox_id IS NOT NULL;

-- Tenant scoping index for dashboards & audit
CREATE INDEX IF NOT EXISTS idx_sandbox_leases_org_repo ON sandbox_leases(organization_id, repository_id);

-- Idle-kill query: find leases ready to be killed (sparse)
CREATE INDEX IF NOT EXISTS idx_sandbox_leases_kill_at ON sandbox_leases(kill_at, sandbox_id) WHERE kill_at IS NOT NULL;

-- Multi-tenant Row-Level Security (Master Rule 4.4)
ALTER TABLE sandbox_leases ENABLE ROW LEVEL SECURITY;
ALTER TABLE sandbox_leases FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_sandbox_leases ON sandbox_leases;
CREATE POLICY tenant_isolation_sandbox_leases ON sandbox_leases
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id IS NULL
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR organization_id = NULLIF(current_setting('app.current_workspace_id', true), '')::uuid
    );

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON sandbox_leases TO scandrix_app;
    END IF;
END $$;`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 026_sandbox_leases.sql.
func (m *Migration026SandboxLeases) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS sandbox_leases CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
