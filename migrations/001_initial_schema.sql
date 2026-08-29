-- Scandrix Multi-Tenant Schema & Row-Level Security Migration 001
-- Clean-Room Enterprise Design

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Workspaces Tenant Boundaries
CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Account Profiles
CREATE TABLE IF NOT EXISTS account_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'MEMBER',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, email)
);

-- Monitored Repositories
CREATE TABLE IF NOT EXISTS tracked_repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    external_id VARCHAR(128) NOT NULL,
    namespace_path VARCHAR(255) NOT NULL,
    default_branch VARCHAR(128) NOT NULL DEFAULT 'main',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workspace_id, provider, external_id)
);

-- Pull Request Reviews
CREATE TABLE IF NOT EXISTS pull_request_reviews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    pull_number INT NOT NULL,
    title VARCHAR(512) NOT NULL,
    head_sha VARCHAR(64) NOT NULL,
    base_sha VARCHAR(64) NOT NULL,
    author_username VARCHAR(128) NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'RECEIVED',
    findings_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

-- Review Findings
CREATE TABLE IF NOT EXISTS code_findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id UUID NOT NULL REFERENCES pull_request_reviews(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    file_path VARCHAR(1024) NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    severity VARCHAR(32) NOT NULL,
    category VARCHAR(64) NOT NULL,
    title VARCHAR(512) NOT NULL,
    description TEXT NOT NULL,
    remediation TEXT NOT NULL,
    suggested_diff TEXT,
    fingerprint VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Transactional Outbox for Guaranteed Delivery
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    event_type VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

-- Review Rules
CREATE TABLE IF NOT EXISTS review_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    severity VARCHAR(32) NOT NULL DEFAULT 'MEDIUM',
    rule_type VARCHAR(64) NOT NULL,
    rule_content TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indices
CREATE INDEX IF NOT EXISTS idx_reviews_workspace_repo ON pull_request_reviews(workspace_id, repository_id);
CREATE INDEX IF NOT EXISTS idx_findings_review ON code_findings(review_id);
CREATE INDEX IF NOT EXISTS idx_findings_workspace ON code_findings(workspace_id);
CREATE INDEX IF NOT EXISTS idx_outbox_status_created ON outbox_events(status, created_at) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_rules_workspace ON review_rules(workspace_id);

-- Enable and Force Row-Level Security (Master Rule 4.4)
ALTER TABLE account_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_profiles FORCE ROW LEVEL SECURITY;

ALTER TABLE tracked_repositories ENABLE ROW LEVEL SECURITY;
ALTER TABLE tracked_repositories FORCE ROW LEVEL SECURITY;

ALTER TABLE pull_request_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE pull_request_reviews FORCE ROW LEVEL SECURITY;

ALTER TABLE code_findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_findings FORCE ROW LEVEL SECURITY;

ALTER TABLE outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox_events FORCE ROW LEVEL SECURITY;

ALTER TABLE review_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE review_rules FORCE ROW LEVEL SECURITY;

-- Tenant Isolation Policies via app.current_tenant_id session config
CREATE POLICY tenant_isolation_profiles ON account_profiles
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_repos ON tracked_repositories
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_reviews ON pull_request_reviews
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_findings ON code_findings
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_outbox ON outbox_events
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_rules ON review_rules
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
