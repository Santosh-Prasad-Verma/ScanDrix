package migrations

import (
	"context"
)

// Migration028PlatformPullRequests represents database migration 028_platform_pull_requests.
type Migration028PlatformPullRequests struct{}

// Version returns the unique migration version string.
func (m *Migration028PlatformPullRequests) Version() string {
	return "028"
}

// Name returns the descriptive name of the migration.
func (m *Migration028PlatformPullRequests) Name() string {
	return "028_platform_pull_requests"
}

// Up applies the schema changes defined in 028_platform_pull_requests.sql.
func (m *Migration028PlatformPullRequests) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 028: Platform Pull Requests Rich Document Store
-- Replaces external Mongo document storage with PostgreSQL JSONB
-- High-throughput GIN indexing, atomic batch writes, and RLS isolation

CREATE TABLE IF NOT EXISTS platform_pull_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id VARCHAR(255) NOT NULL,
    number INT NOT NULL,
    title VARCHAR(512) NOT NULL,
    status VARCHAR(64) NOT NULL DEFAULT 'OPEN',
    merged BOOLEAN NOT NULL DEFAULT false,
    heavy BOOLEAN NOT NULL DEFAULT false,
    is_draft BOOLEAN NOT NULL DEFAULT false,
    provider VARCHAR(32) NOT NULL,
    url VARCHAR(1024) NOT NULL DEFAULT '',
    base_branch_ref VARCHAR(255) NOT NULL DEFAULT '',
    head_branch_ref VARCHAR(255) NOT NULL DEFAULT '',
    user_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    repository_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    files JSONB NOT NULL DEFAULT '[]'::jsonb,
    commits JSONB NOT NULL DEFAULT '[]'::jsonb,
    suggestions_by_pr JSONB NOT NULL DEFAULT '[]'::jsonb,
    pr_level_suggestions JSONB NOT NULL DEFAULT '[]'::jsonb,
    total_added INT NOT NULL DEFAULT 0,
    total_deleted INT NOT NULL DEFAULT 0,
    total_changes INT NOT NULL DEFAULT 0,
    synced_embedded_suggestions BOOLEAN NOT NULL DEFAULT false,
    synced_with_issues BOOLEAN NOT NULL DEFAULT false,
    opened_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_platform_prs_ws_repo_num UNIQUE (workspace_id, repository_id, number)
);

-- Point lookup index on workspace, repository, and pull request number
CREATE INDEX IF NOT EXISTS idx_platform_prs_ws_repo_num 
    ON platform_pull_requests (workspace_id, repository_id, number);

-- GIN index on files for deep suggestion, line, and path querying
CREATE INDEX IF NOT EXISTS idx_platform_prs_files_gin 
    ON platform_pull_requests USING GIN (files);

-- GIN index on commits for commit SHA and author lookups
CREATE INDEX IF NOT EXISTS idx_platform_prs_commits_gin 
    ON platform_pull_requests USING GIN (commits);

-- GIN index on user_data for developer mapping and token queries
CREATE INDEX IF NOT EXISTS idx_platform_prs_user_data_gin 
    ON platform_pull_requests USING GIN (user_data);

-- Row-Level Security (RLS) Tenant Isolation
ALTER TABLE platform_pull_requests ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_platform_prs ON platform_pull_requests;
CREATE POLICY tenant_isolation_platform_prs ON platform_pull_requests
    FOR ALL USING (
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR current_setting('app.is_system_worker', true) = 'true'
        OR current_user IN ('postgres', 'supabase_admin', 'service_role')
    );`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 028_platform_pull_requests.sql.
func (m *Migration028PlatformPullRequests) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS platform_pull_requests CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
