-- Migration 040: Row-Level Security for the Drixy rules tables.
--
-- AUDIT_REMEDIATION.md F-37
--
-- drixy_rules and drixy_rule_likes were the only tenant-scoped tables in the
-- schema with no RLS, while 41 others had it FORCEd. drixy_rules holds each
-- workspace's entire rule configuration.
--
-- Two things this migration deliberately does NOT do:
--
--   * It does not change cli_auth_sessions. That table backs the RFC 8628
--     device flow, which is unauthenticated by definition and has no tenant
--     context until after the code is redeemed, so a tenant policy would
--     reject every legitimate flow. Applying it requires moving that state
--     behind a tenant-scoped store first (the F-28 work). Leaving it open here
--     is a known, documented gap rather than a silent one.
--
--   * It does not assume organization_id is a uuid. drixy_rules.organization_id
--     is VARCHAR(255), so the policies compare as TEXT. Casting the column to
--     uuid makes every query fail with "operator does not exist: character
--     varying = uuid", which is worse than having no policy at all because it
--     looks like an outage rather than a permissions problem.
--
-- Both tables are granted to the application role; RLS then filters rows.

-- ---------------------------------------------------------------- drixy_rules
ALTER TABLE drixy_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE drixy_rules FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_drixy_rules ON drixy_rules;
CREATE POLICY tenant_isolation_drixy_rules ON drixy_rules
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::text
        OR organization_id = NULLIF(current_setting('app.current_workspace_id', true), '')::text
    )
    WITH CHECK (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::text
        OR organization_id = NULLIF(current_setting('app.current_workspace_id', true), '')::text
    );

-- --------------------------------------------------------- drixy_rule_likes
-- The table had no tenant column at all, so a policy had nothing to compare
-- against. organization_id is added and backfilled by resolving each rule_id
-- against the owning workspace's drixy_rules document. Rows that cannot be
-- resolved keep a NULL organization_id, which no tenant policy admits, so
-- they are readable by the system worker only rather than leaking to everyone.
ALTER TABLE drixy_rule_likes ADD COLUMN IF NOT EXISTS organization_id TEXT;

UPDATE drixy_rule_likes l
SET organization_id = r.organization_id
FROM drixy_rules r
WHERE l.organization_id IS NULL
  AND EXISTS (
      SELECT 1
      FROM jsonb_array_elements(COALESCE(r.rules, '[]'::jsonb)) e
      WHERE e->>'uuid' = l.rule_id
  );

CREATE INDEX IF NOT EXISTS idx_drixy_rule_likes_org
    ON drixy_rule_likes(organization_id);

ALTER TABLE drixy_rule_likes ENABLE ROW LEVEL SECURITY;
ALTER TABLE drixy_rule_likes FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_drixy_rule_likes ON drixy_rule_likes;
CREATE POLICY tenant_isolation_drixy_rule_likes ON drixy_rule_likes
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::text
        OR organization_id = NULLIF(current_setting('app.current_workspace_id', true), '')::text
    )
    WITH CHECK (
        current_setting('app.is_system_worker', true) = 'true'
        OR organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::text
        OR organization_id = NULLIF(current_setting('app.current_workspace_id', true), '')::text
    );
