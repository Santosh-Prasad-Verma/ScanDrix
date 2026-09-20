package migrations

import (
	"context"
)

// Migration021SystemBypassUsersRls represents database migration 021_system_bypass_users_rls.
type Migration021SystemBypassUsersRls struct{}

// Version returns the unique migration version string.
func (m *Migration021SystemBypassUsersRls) Version() string {
	return "021"
}

// Name returns the descriptive name of the migration.
func (m *Migration021SystemBypassUsersRls) Name() string {
	return "021_system_bypass_users_rls"
}

// Up applies the schema changes defined in 021_system_bypass_users_rls.sql.
func (m *Migration021SystemBypassUsersRls) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 021: Add system worker bypass to users table RLS policy
-- Allows pre-auth system queries (login, password reset, token validation)
-- to resolve user records while preserving strict tenant isolation for authenticated sessions.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'users') THEN
        DROP POLICY IF EXISTS tenant_isolation_users ON users;
        CREATE POLICY tenant_isolation_users ON users
            FOR ALL
            USING (
                current_setting('app.is_system_worker', true) = 'true'
                OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
            );
    END IF;
END $$;`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 021_system_bypass_users_rls.sql.
func (m *Migration021SystemBypassUsersRls) Down(ctx context.Context, exec SQLExecutor) error {
	query := `-- Rollback system bypass users RLS
SELECT 1;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
