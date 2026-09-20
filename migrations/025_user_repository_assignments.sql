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

GRANT SELECT, INSERT, UPDATE, DELETE ON user_repository_assignments TO scandrix_app;
