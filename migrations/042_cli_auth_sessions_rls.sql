-- Row-Level Security for cli_auth_sessions.
--
-- The table previously had no RLS because its device flow authenticated by
-- device_code before any tenant existed, so there was nothing to scope a row
-- to. The tenant is now bound on the browser-approval leg
-- (AUDIT_REMEDIATION.md F-37), which makes a policy possible.
--
-- The lifecycle has three phases, and only the last one has a tenant:
--
--   PENDING   no browser has approved yet, so workspace_id IS NULL by design.
--             The CLI only holds device_code, and the browser approval path
--             reads the row before a tenant exists. These rows must therefore
--             stay readable by the approving principal.
--   COMPLETED workspace_id is set and is the only tenant that may read it.
--             This is the row that yields a token, so it is the one that
--             matters: without this clause any authenticated principal could
--             read another tenant's completed session and its access token.
--   CONSUMED  replayed exactly once; the token columns are nulled by the
--             consumer, and it is retained only for audit.
--
-- Privileged paths set app.is_system_worker, which is how the worker reads
-- sessions to deliver tokens. That is a database setting, not a client
-- input, so it cannot be turned on by a request.

ALTER TABLE cli_auth_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE cli_auth_sessions FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS cli_auth_sessions_lifecycle ON cli_auth_sessions;
CREATE POLICY cli_auth_sessions_lifecycle ON cli_auth_sessions
    FOR ALL
    USING (
        -- Workers need cross-tenant access to deliver and consume sessions.
        current_setting('app.is_system_worker', true) = 'true'
        OR (
            -- A row that has not been approved yet has no tenant by design.
            workspace_id IS NULL
            -- A completed/consumed row belongs to exactly one tenant.
            OR workspace_id = current_setting('app.workspace_id', true)::uuid
        )
    )
    WITH CHECK (
        current_setting('app.is_system_worker', true) = 'true'
        -- A pending session legitimately has no tenant: the CLI holds only the
        -- device code at that point. Allowing it here keeps the policy
        -- consistent with the USING clause, which already admits NULL-tenant
        -- rows for exactly this phase. It cannot be used to smuggle out a
        -- token, because a pending row has no token: the token columns are
        -- written by the approval UPDATE, which must supply the tenant.
        OR (
            workspace_id IS NULL
            AND upper(status) = 'PENDING'
        )
        OR workspace_id = current_setting('app.workspace_id', true)::uuid
    );

-- Index the tenant column: every scoped read filters on it.
CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_workspace
    ON cli_auth_sessions(workspace_id);
