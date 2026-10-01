-- Migration 041: access-token revocation.
--
-- AUDIT_REMEDIATION.md F-18
--
-- Logout only marked the refresh token used and cleared cookies. It never
-- touched the access token, so a stolen bearer token kept working for its
-- entire lifetime. The middleware already had a revocation hook
-- (Authenticator.SetRevocationChecker) but nothing ever called it, so the
-- protection existed only as dead code that read as if it were enforced.
--
-- Model
-- -----
-- A row here is a *cutoff*, not a single token: every access token for that
-- user whose issued-at (iat) is less than or equal to the cutoff is rejected.
-- Logging out therefore kills every session token outstanding at that moment
-- in one insert, including ones already copied elsewhere, without having to
-- track each token individually.
--
-- Tokens issued *after* the cutoff keep working, which is the intended
-- behaviour: a refresh that races with the logout mints a genuinely new
-- session.
--
-- The check has to run before the middleware establishes any tenant context
-- (the revocation lookup happens before WithWorkspaceContext), so a tenant
-- policy has nothing to compare against. RLS is therefore enforced with a
-- policy that admits only the system worker. The application reaches the table
-- exclusively through ExecAsSystem, which sets app.is_system_worker; the
-- runtime role gets no tenant access at all.

CREATE TABLE IF NOT EXISTS revoked_access_tokens (
    user_id    UUID        NOT NULL,
    -- Access-token iat boundary. Tokens with iat <= this value are rejected.
    issued_at  BIGINT      NOT NULL,
    -- Tokens past this are expired anyway; the row is only needed until then.
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, issued_at)
);

-- Supports the cleanup sweep that drops rows whose tokens have all expired.
CREATE INDEX IF NOT EXISTS idx_revoked_access_tokens_expires_at
    ON revoked_access_tokens (expires_at);

ALTER TABLE revoked_access_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE revoked_access_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS system_only_revoked_access_tokens ON revoked_access_tokens;
CREATE POLICY system_only_revoked_access_tokens ON revoked_access_tokens
    FOR ALL
    USING (current_setting('app.is_system_worker', true) = 'true')
    WITH CHECK (current_setting('app.is_system_worker', true) = 'true');

-- Guarded: the least-privilege runtime role is provisioned out-of-band (see
-- migrations/ops/002_least_privilege_runtime_role.sql) and does not exist in
-- CI, where an unguarded GRANT raised 42704 and rolled the whole migration
-- back. With no runtime role there is nothing to grant to; the RLS policy
-- above still applies to whatever role ends up reading the table.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_runtime') THEN
        RAISE NOTICE 'Skipping grants to scandrix_runtime: the role does not exist.';
        RETURN;
    END IF;
    EXECUTE 'GRANT SELECT, INSERT, DELETE ON revoked_access_tokens TO scandrix_runtime';
END
$$;
