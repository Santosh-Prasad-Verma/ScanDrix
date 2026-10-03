-- =============================================================================
-- Migration 035: persistent SCIM groups
--
-- Problem
-- -------
-- SCIM users were persisted (internal/database/scim_repository.go) but SCIM
-- groups lived in a map on the SCIMService struct. That meant:
--   * every group vanished on restart, so an IdP that reconciles groups would
--     see them all disappear and recreate them on every deploy;
--   * with more than one API replica, group state diverged between pods;
--   * group membership, which is how IdPs grant entitlements, was never durable.
--
-- Design
-- ------
-- Groups and their members are stored relationally rather than as a JSON blob
-- so membership can be joined against users, and so a member removal is a
-- single DELETE rather than a read-modify-write of the whole document.
--
-- RLS follows the same shape as sso_configs (migration 031): a system worker
-- may read across tenants (the outbox relay and background jobs), while a
-- request-scoped path may only touch its own workspace. WITH CHECK is
-- tenant-only, so a caller cannot insert a group into another workspace even
-- while holding the system flag.
-- =============================================================================

CREATE TABLE IF NOT EXISTS scim_groups (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id  uuid        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    scim_id       text        NOT NULL,
    display_name  text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, scim_id)
);

CREATE TABLE IF NOT EXISTS scim_group_members (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid       NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    group_id    uuid        NOT NULL REFERENCES scim_groups(id) ON DELETE CASCADE,
    -- SCIM "value" of the member: a user scim_id, or an external group id when
    -- the IdP nests groups.
    member_ref  text        NOT NULL,
    display_ref text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id, member_ref)
);

CREATE INDEX IF NOT EXISTS idx_scim_groups_workspace ON scim_groups (workspace_id);
CREATE INDEX IF NOT EXISTS idx_scim_group_members_group ON scim_group_members (group_id);
CREATE INDEX IF NOT EXISTS idx_scim_group_members_workspace ON scim_group_members (workspace_id);

ALTER TABLE scim_groups ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_groups FORCE ROW LEVEL SECURITY;
ALTER TABLE scim_group_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE scim_group_members FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_scim_groups ON scim_groups;
CREATE POLICY tenant_isolation_scim_groups ON scim_groups
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_scim_group_members ON scim_group_members;
CREATE POLICY tenant_isolation_scim_group_members ON scim_group_members
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
