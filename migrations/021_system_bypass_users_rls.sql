-- Migration 021: Add system worker bypass to users table RLS policy
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
END $$;
