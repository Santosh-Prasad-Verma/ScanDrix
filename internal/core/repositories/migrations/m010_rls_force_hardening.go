package migrations

import (
	"context"
)

// Migration010RlsForceHardening represents database migration 010_rls_force_hardening.
type Migration010RlsForceHardening struct{}

// Version returns the unique migration version string.
func (m *Migration010RlsForceHardening) Version() string {
	return "010"
}

// Name returns the descriptive name of the migration.
func (m *Migration010RlsForceHardening) Name() string {
	return "010_rls_force_hardening"
}

// Up applies the schema changes defined in 010_rls_force_hardening.sql.
func (m *Migration010RlsForceHardening) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 010: Complete RLS Force Hardening on All Tenant Tables
-- Guarantees table owners and migration users cannot bypass tenant boundaries.

-- 1. FORCE ROW LEVEL SECURITY on billing, attestations, finding tickets, and pm configs
ALTER TABLE billing_transactions FORCE ROW LEVEL SECURITY;
ALTER TABLE review_attestations FORCE ROW LEVEL SECURITY;
ALTER TABLE finding_tickets FORCE ROW LEVEL SECURITY;
ALTER TABLE pm_auto_ticket_configs FORCE ROW LEVEL SECURITY;

-- 2. Ensure RLS on users table (Tenant Boundary Hardening)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users') THEN
        ALTER TABLE users ENABLE ROW LEVEL SECURITY;
        ALTER TABLE users FORCE ROW LEVEL SECURITY;

        DROP POLICY IF EXISTS tenant_isolation_users ON users;
        CREATE POLICY tenant_isolation_users ON users
            FOR ALL
            USING (organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
    END IF;
END $$;`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 010_rls_force_hardening.sql.
func (m *Migration010RlsForceHardening) Down(ctx context.Context, exec SQLExecutor) error {
	query := `-- Rollback RLS force hardening
SELECT 1;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
