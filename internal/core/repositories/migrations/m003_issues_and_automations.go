package migrations

import (
	"context"
)

// Migration003IssuesAndAutomations represents database migration 003_issues_and_automations.
type Migration003IssuesAndAutomations struct{}

// Version returns the unique migration version string.
func (m *Migration003IssuesAndAutomations) Version() string {
	return "003"
}

// Name returns the descriptive name of the migration.
func (m *Migration003IssuesAndAutomations) Name() string {
	return "003_issues_and_automations"
}

// Up applies the schema changes defined in 003_issues_and_automations.sql.
func (m *Migration003IssuesAndAutomations) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 003: Enterprise Issues, Team Automations & Persistent AST Graph
-- Fully isolated with PostgreSQL Row-Level Security (RLS)

-- 1. Tracked Issues across Commits & Branches
CREATE TABLE IF NOT EXISTS tracked_issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    title VARCHAR(512) NOT NULL,
    description TEXT NOT NULL,
    file_path VARCHAR(1024) NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    severity VARCHAR(32) NOT NULL,
    category VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'OPEN',
    origin_review_id UUID REFERENCES pull_request_reviews(id) ON DELETE SET NULL,
    remediation TEXT NOT NULL DEFAULT '',
    fingerprint VARCHAR(128) NOT NULL,
    external_issue_url VARCHAR(1024),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tracked_issues_workspace ON tracked_issues(workspace_id);
CREATE INDEX IF NOT EXISTS idx_tracked_issues_repo ON tracked_issues(repository_id);
CREATE INDEX IF NOT EXISTS idx_tracked_issues_status ON tracked_issues(workspace_id, status);
CREATE INDEX IF NOT EXISTS idx_tracked_issues_fp ON tracked_issues(workspace_id, fingerprint);

ALTER TABLE tracked_issues ENABLE ROW LEVEL SECURITY;
ALTER TABLE tracked_issues FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_tracked_issues ON tracked_issues;
CREATE POLICY tenant_isolation_tracked_issues ON tracked_issues
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 2. Team Workflow Automations
CREATE TABLE IF NOT EXISTS workflow_automations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    trigger VARCHAR(64) NOT NULL,
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workflow_automations_ws ON workflow_automations(workspace_id);

ALTER TABLE workflow_automations ENABLE ROW LEVEL SECURITY;
ALTER TABLE workflow_automations FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_workflow_automations ON workflow_automations;
CREATE POLICY tenant_isolation_workflow_automations ON workflow_automations
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 3. Automation Execution Audit Logs
CREATE TABLE IF NOT EXISTS automation_execution_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID NOT NULL REFERENCES workflow_automations(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    matched BOOLEAN NOT NULL DEFAULT TRUE,
    actions_executed JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_automation_logs_ws ON automation_execution_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_automation_logs_rule ON automation_execution_logs(rule_id);

ALTER TABLE automation_execution_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_execution_logs FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_automation_execution_logs ON automation_execution_logs;
CREATE POLICY tenant_isolation_automation_execution_logs ON automation_execution_logs
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 4. Code Architecture AST Graph Nodes
CREATE TABLE IF NOT EXISTS code_ast_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    kind VARCHAR(32) NOT NULL,
    symbol_name VARCHAR(255) NOT NULL,
    file_path VARCHAR(1024) NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    language VARCHAR(32) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_code_ast_nodes_repo_symbol ON code_ast_nodes(repository_id, symbol_name);
CREATE INDEX IF NOT EXISTS idx_code_ast_nodes_repo_file ON code_ast_nodes(repository_id, file_path);


-- 5. Code Architecture AST Graph Edges (Call Graph, Imports)
CREATE TABLE IF NOT EXISTS code_ast_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    from_node_id UUID NOT NULL REFERENCES code_ast_nodes(id) ON DELETE CASCADE,
    to_node_id UUID NOT NULL REFERENCES code_ast_nodes(id) ON DELETE CASCADE,
    kind VARCHAR(32) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_code_ast_edges_to ON code_ast_edges(repository_id, to_node_id);
CREATE INDEX IF NOT EXISTS idx_code_ast_edges_from ON code_ast_edges(repository_id, from_node_id);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 003_issues_and_automations.sql.
func (m *Migration003IssuesAndAutomations) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS code_ast_edges, code_ast_nodes, automation_execution_logs, workflow_automations, tracked_issues CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
