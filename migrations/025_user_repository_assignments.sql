-- Migration 025: User Repository Assignments for Fine-Grained Enterprise RBAC
-- Provisions persistent table for per-user repository restrictions with tenant isolation (RLS)

CREATE TABLE IF NOT EXISTS user_repository_assignments (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    repository_ids UUID[] NOT NULL DEFAULT '{}',
    assigned_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_repo_assignment UNIQUE (workspace_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_repo_assignments_ws_user ON user_repository_assignments(workspace_id, user_id);

ALTER TABLE user_repository_assignments ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_repository_assignments FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_user_repo_assignments ON user_repository_assignments;
CREATE POLICY tenant_isolation_user_repo_assignments ON user_repository_assignments
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR workspace_id = NULLIF(current_setting('app.current_workspace_id', true), '')::uuid
    );


-- Skipped when the least-privilege runtime role has not been provisioned (CI,
-- or provisioned out-of-band): granting to a non-existent role raises 42704 and
-- rolls the whole migration back. With no runtime role there is nothing to
-- grant to. See migrations/ops/002_least_privilege_runtime_role.sql.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_app') THEN
        RAISE NOTICE 'Skipping grants to scandrix_app: the role does not exist.';
        RETURN;
    END IF;
    EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON user_repository_assignments TO scandrix_app;';
END
$$;
