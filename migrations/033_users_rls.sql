-- =============================================================================
-- Migration 032: Row Level Security on `users`
--
-- Why this exists
-- ---------------
-- `users` was the largest tenant-scoped table in the schema with no RLS. Every
-- other tenant-owned table already had `tenant_isolation_*` policies, so a
-- compromised or buggy code path in any one service could read every user
-- record in the deployment. Users hold password hashes, roles and status, so
-- that is the single highest-value table to lock down.
--
-- Why a plain policy is enough here
-- --------------------------------
-- The pre-authentication lookups (login by email, session lookup by uuid, and
-- email-domain to organization resolution) all run through
-- `Client.ExecAsSystem`, which sets `app.is_system_worker`. The policy below
-- admits those, so enabling RLS does not break the login path. That was
-- verified before this migration was written; see the test in
-- internal/database/users_rls_test.go.
--
-- Residual risk, stated plainly
-- -----------------------------
-- `app.is_system_worker` is a broad bypass: any code path that sets it can read
-- any user, exactly as it can for the other 40 RLS-protected tables. Narrowing
-- that requires SECURITY DEFINER lookup functions plus revoking table-level
-- grants, which is a larger change and is tracked separately. Until then the
-- guarantee is "a request-scoped code path cannot read another tenant's users",
-- not "no code path can".
-- =============================================================================

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_users ON users;

CREATE POLICY tenant_isolation_users ON users
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
