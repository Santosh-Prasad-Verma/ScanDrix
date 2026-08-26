-- ============================================================================
-- CodeHound Enterprise Security & Verification Engine
-- Migration 000001: Initial Core Schema (23 Tables, Enums, RLS, Partitions)
-- Target DBMS: PostgreSQL 18+
-- ============================================================================

-- 1. Enable Required Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- 2. Define Custom Enums
CREATE TYPE tenant_plan_enum AS ENUM ('COMMUNITY', 'TEAM', 'ENTERPRISE');
CREATE TYPE tenant_status_enum AS ENUM ('ACTIVE', 'SUSPENDED', 'PENDING_DELETION');

CREATE TYPE scan_type_enum AS ENUM ('FULL', 'DIFF_AWARE', 'SECURITY_ONLY', 'PERFORMANCE');
CREATE TYPE scan_status_enum AS ENUM ('QUEUED', 'INITIALIZING', 'INGESTING', 'SCANNING', 'VERIFYING', 'COMPLETED', 'FAILED', 'ABORTED');

CREATE TYPE finding_severity_enum AS ENUM ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW', 'INFO');
CREATE TYPE finding_state_enum AS ENUM ('SUSPECTED', 'VERIFIED_PROVEN', 'RESOLVED', 'FALSE_POSITIVE', 'ACCEPTED_RISK');
CREATE TYPE finding_category_enum AS ENUM ('SECURITY_VULN', 'LOGIC_BUG', 'SECRET_LEAK', 'PERFORMANCE_BOTTLENECK', 'BREAKING_API_DIFF', 'IAC_MISCONFIG', 'LICENSE_CONFLICT');

CREATE TYPE execution_type_enum AS ENUM ('AST_PARSER', 'STATIC_SAST', 'AI_REASONING', 'SANDBOX_TEST', 'MUTATION_TEST', 'DYNAMIC_DAST', 'K6_LOAD_SURGE', 'PATCH_VERIFICATION');
CREATE TYPE sandbox_tier_enum AS ENUM ('TIER_A_GVISOR', 'TIER_B_FIRECRACKER_MICROVM', 'TIER_C_NETWORK_LAB');
CREATE TYPE execution_status_enum AS ENUM ('QUEUED', 'RUNNING', 'PASSED', 'FAILED', 'TIMED_OUT', 'RESOURCE_EXCEEDED');

CREATE TYPE patch_status_enum AS ENUM ('PROPOSED', 'SANDBOX_VALIDATING', 'VERIFIED_PASSING', 'VERIFICATION_FAILED', 'PR_OPENED', 'MERGED', 'REJECTED');

-- ----------------------------------------------------------------------------
-- 3. Tenant, Identity & Access Layer
-- ----------------------------------------------------------------------------

-- Table 1: Tenants
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    plan tenant_plan_enum NOT NULL DEFAULT 'TEAM',
    status tenant_status_enum NOT NULL DEFAULT 'ACTIVE',
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

-- Table 2: Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    external_auth_id VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL DEFAULT 'DEVELOPER',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_tenant_user UNIQUE (tenant_id, external_auth_id),
    CONSTRAINT uq_tenant_email UNIQUE (tenant_id, email)
);
CREATE INDEX idx_users_tenant ON users (tenant_id);

-- Table 3: API Keys
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(128) NOT NULL,
    key_prefix VARCHAR(16) NOT NULL,
    key_hash VARCHAR(128) NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{"read:audit", "write:audit"}',
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_key_hash UNIQUE (key_hash)
);
CREATE INDEX idx_api_keys_prefix ON api_keys (key_prefix) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id);

-- ----------------------------------------------------------------------------
-- 4. Projects, Policies & Repositories
-- ----------------------------------------------------------------------------

-- Table 4: Policy Profiles
CREATE TABLE policy_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    rules JSONB NOT NULL DEFAULT '{"block_on_critical": true, "require_mutation_score": 0.80}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_tenant_policy_name UNIQUE (tenant_id, name)
);
CREATE INDEX idx_policy_profiles_tenant ON policy_profiles (tenant_id);

-- Table 5: Projects
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    policy_profile_id UUID REFERENCES policy_profiles(id) ON DELETE SET NULL,
    slug VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    default_branch VARCHAR(128) NOT NULL DEFAULT 'main',
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_tenant_project_slug UNIQUE (tenant_id, slug)
);
CREATE INDEX idx_projects_tenant ON projects (tenant_id);

-- Table 6: Repositories
CREATE TABLE repositories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    external_repo_id VARCHAR(128),
    clone_url TEXT NOT NULL,
    default_branch VARCHAR(128) NOT NULL DEFAULT 'main',
    last_indexed_commit_sha VARCHAR(64),
    last_indexed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_project_repo UNIQUE (project_id, provider, external_repo_id)
);
CREATE INDEX idx_repositories_project ON repositories (project_id);
CREATE INDEX idx_repositories_tenant ON repositories (tenant_id);

-- ----------------------------------------------------------------------------
-- 5. Code Intelligence & AST Symbol Graph
-- ----------------------------------------------------------------------------

-- Table 7: Commits
CREATE TABLE commits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    sha VARCHAR(64) NOT NULL,
    parent_shas TEXT[] NOT NULL DEFAULT '{}',
    author_name TEXT,
    author_email TEXT,
    commit_message TEXT,
    committed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_repo_commit UNIQUE (repository_id, sha)
);
CREATE INDEX idx_commits_repo_sha ON commits (repository_id, sha);

-- Table 8: Code Files
CREATE TABLE code_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    commit_id UUID NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
    file_path TEXT NOT NULL,
    language VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    object_store_uri TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_commit_filepath UNIQUE (commit_id, file_path)
);
CREATE INDEX idx_code_files_lookup ON code_files (repository_id, commit_id, file_path);

-- Table 9: Code Symbols
CREATE TABLE code_symbols (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_file_id UUID NOT NULL REFERENCES code_files(id) ON DELETE CASCADE,
    symbol_key TEXT NOT NULL,
    name VARCHAR(255) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    start_column INT NOT NULL,
    end_column INT NOT NULL,
    signature TEXT,
    visibility VARCHAR(32) DEFAULT 'PUBLIC',
    tainted_inputs TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_symbols_file_name ON code_symbols (code_file_id, name);
CREATE INDEX idx_symbols_key ON code_symbols (symbol_key);

-- Table 10: Code Edges
CREATE TABLE code_edges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    commit_id UUID NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
    from_symbol_id UUID NOT NULL REFERENCES code_symbols(id) ON DELETE CASCADE,
    to_symbol_id UUID NOT NULL REFERENCES code_symbols(id) ON DELETE CASCADE,
    edge_type VARCHAR(32) NOT NULL,
    confidence NUMERIC(3,2) NOT NULL DEFAULT 1.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_edges_from ON code_edges (from_symbol_id, edge_type);
CREATE INDEX idx_edges_to ON code_edges (to_symbol_id, edge_type);

-- ----------------------------------------------------------------------------
-- 6. Dynamic Testing Authorization & Targets
-- ----------------------------------------------------------------------------

-- Table 11: Authorization Grants
CREATE TABLE authorization_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    target_base_url TEXT NOT NULL,
    proof_method VARCHAR(32) NOT NULL,
    challenge_record_name TEXT NOT NULL,
    expected_token_hash VARCHAR(128) NOT NULL,
    is_verified BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    valid_until TIMESTAMPTZ NOT NULL,
    approved_by_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_project_target_url UNIQUE (project_id, target_base_url)
);
CREATE INDEX idx_auth_grants_tenant ON authorization_grants (tenant_id);

-- Table 12: Test Targets
CREATE TABLE test_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    authorization_grant_id UUID NOT NULL REFERENCES authorization_grants(id) ON DELETE RESTRICT,
    environment VARCHAR(32) NOT NULL DEFAULT 'STAGING',
    base_url TEXT NOT NULL,
    max_vu_ceiling INT NOT NULL DEFAULT 50000,
    max_duration_seconds INT NOT NULL DEFAULT 600,
    scope_path_allowlist TEXT[] NOT NULL DEFAULT '{"/api/v1/*"}',
    headers_vault_ref TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_test_targets_project ON test_targets (project_id);

-- ----------------------------------------------------------------------------
-- 7. Scans, Findings, Evidence & Patches
-- ----------------------------------------------------------------------------

-- Table 13: Scans
CREATE TABLE scans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    commit_id UUID NOT NULL REFERENCES commits(id) ON DELETE RESTRICT,
    scan_type scan_type_enum NOT NULL DEFAULT 'FULL',
    status scan_status_enum NOT NULL DEFAULT 'QUEUED',
    health_score INT CHECK (health_score BETWEEN 0 AND 100),
    initiated_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    workflow_id VARCHAR(128) NOT NULL,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_scans_project_status ON scans (project_id, status);
CREATE INDEX idx_scans_tenant ON scans (tenant_id);

-- Table 14: Findings
CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    canonical_key VARCHAR(128) NOT NULL,
    category finding_category_enum NOT NULL,
    severity finding_severity_enum NOT NULL,
    state finding_state_enum NOT NULL DEFAULT 'SUSPECTED',
    confidence NUMERIC(3,2) NOT NULL DEFAULT 0.50,
    blast_radius_score INT CHECK (blast_radius_score BETWEEN 0 AND 100),
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    primary_file TEXT NOT NULL,
    primary_line INT NOT NULL,
    cwe_id VARCHAR(32),
    cve_id VARCHAR(32),
    first_seen_scan_id UUID NOT NULL REFERENCES scans(id) ON DELETE RESTRICT,
    last_seen_scan_id UUID NOT NULL REFERENCES scans(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_project_canonical_finding UNIQUE (project_id, canonical_key)
);
CREATE INDEX idx_findings_project_sev_state ON findings (project_id, severity, state);
CREATE INDEX idx_findings_tenant ON findings (tenant_id);

-- Table 15: Finding Occurrences (Partitioned Table)
CREATE TABLE finding_occurrences (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    finding_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    commit_sha VARCHAR(64) NOT NULL,
    file_path TEXT NOT NULL,
    start_line INT NOT NULL,
    end_line INT NOT NULL,
    code_snippet_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Partitions for finding_occurrences
CREATE TABLE finding_occurrences_y2026m08 PARTITION OF finding_occurrences
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');
CREATE TABLE finding_occurrences_y2026m09 PARTITION OF finding_occurrences
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');
CREATE TABLE finding_occurrences_y2026m10 PARTITION OF finding_occurrences
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

-- Table 16: Evidence
CREATE TABLE evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id UUID NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    evidence_type VARCHAR(64) NOT NULL,
    source_analyzer VARCHAR(64) NOT NULL,
    strength VARCHAR(32) NOT NULL DEFAULT 'HIGH',
    summary TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifact_uri TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_evidence_finding ON evidence (finding_id);

-- Table 17: Patches
CREATE TABLE patches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    finding_id UUID NOT NULL REFERENCES findings(id) ON DELETE CASCADE,
    scan_id UUID NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    status patch_status_enum NOT NULL DEFAULT 'PROPOSED',
    unified_diff TEXT NOT NULL,
    diff_hash VARCHAR(64) NOT NULL,
    mutation_score NUMERIC(3,2),
    pr_url TEXT,
    pr_number INT,
    applied_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_patches_finding ON patches (finding_id);
CREATE INDEX idx_patches_scan ON patches (scan_id);

-- ----------------------------------------------------------------------------
-- 8. Executions, Tests, Models & Artifacts
-- ----------------------------------------------------------------------------

-- Table 18: Executions (Partitioned Table)
CREATE TABLE executions (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    execution_type execution_type_enum NOT NULL,
    sandbox_tier sandbox_tier_enum NOT NULL,
    status execution_status_enum NOT NULL DEFAULT 'QUEUED',
    exit_code INT,
    duration_ms BIGINT,
    cpu_usage_seconds NUMERIC(10,3),
    peak_memory_bytes BIGINT,
    log_artifact_uri TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Partitions for executions
CREATE TABLE executions_y2026m08 PARTITION OF executions
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');
CREATE TABLE executions_y2026m09 PARTITION OF executions
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');
CREATE TABLE executions_y2026m10 PARTITION OF executions
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

-- Table 19: Test Runs
CREATE TABLE test_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID NOT NULL,
    test_name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PASSED',
    duration_ms INT NOT NULL DEFAULT 0,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    mutation_score NUMERIC(3,2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_test_runs_execution ON test_runs (execution_id);

-- Table 20: Model Runs (Partitioned Table)
CREATE TABLE model_runs (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    scan_id UUID NOT NULL,
    role VARCHAR(64) NOT NULL,
    provider VARCHAR(64) NOT NULL,
    model_name VARCHAR(128) NOT NULL,
    prompt_tokens INT NOT NULL,
    completion_tokens INT NOT NULL,
    total_cost_usd NUMERIC(10,6) NOT NULL,
    latency_ms INT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'COMPLETED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Partitions for model_runs
CREATE TABLE model_runs_y2026m08 PARTITION OF model_runs
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');
CREATE TABLE model_runs_y2026m09 PARTITION OF model_runs
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');
CREATE TABLE model_runs_y2026m10 PARTITION OF model_runs
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

-- Table 21: Artifacts
CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id UUID,
    scan_id UUID,
    artifact_type VARCHAR(64) NOT NULL,
    s3_uri TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_artifacts_scan ON artifacts (scan_id);

-- ----------------------------------------------------------------------------
-- 9. Idempotency & Audit Logging
-- ----------------------------------------------------------------------------

-- Table 22: Idempotency Keys
CREATE TABLE idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    key_hash VARCHAR(128) NOT NULL,
    operation VARCHAR(128) NOT NULL,
    request_payload_hash VARCHAR(64) NOT NULL,
    response_body JSONB,
    status_code INT,
    locked_until TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT uq_tenant_key_op UNIQUE (tenant_id, key_hash, operation)
);
CREATE INDEX idx_idempotency_expiry ON idempotency_keys (expires_at);
CREATE INDEX idx_idempotency_tenant ON idempotency_keys (tenant_id);

-- Table 23: Audit Events (Partitioned Table)
CREATE TABLE audit_events (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID,
    actor_type VARCHAR(32) NOT NULL,
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id UUID NOT NULL,
    ip_address INET,
    user_agent TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Partitions for audit_events
CREATE TABLE audit_events_y2026m08 PARTITION OF audit_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');
CREATE TABLE audit_events_y2026m09 PARTITION OF audit_events
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');
CREATE TABLE audit_events_y2026m10 PARTITION OF audit_events
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

-- ----------------------------------------------------------------------------
-- 10. Multi-Tenant Row Level Security (RLS) Policies
-- ----------------------------------------------------------------------------

ALTER TABLE policy_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_profiles FORCE ROW LEVEL SECURITY;

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects FORCE ROW LEVEL SECURITY;

ALTER TABLE scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE scans FORCE ROW LEVEL SECURITY;

ALTER TABLE findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE findings FORCE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation_policy_profiles ON policy_profiles
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_api_keys ON api_keys
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_projects ON projects
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_scans ON scans
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE POLICY tenant_isolation_findings ON findings
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- ----------------------------------------------------------------------------
-- 11. Application Runtime Role (Enforces RLS)
-- ----------------------------------------------------------------------------

DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'codehound_app') THEN
        CREATE ROLE codehound_app WITH NOBYPASSRLS;
    END IF;
END $$;

GRANT ALL ON SCHEMA public TO codehound_app;
GRANT ALL ON ALL TABLES IN SCHEMA public TO codehound_app;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO codehound_app;
GRANT ALL ON ALL ROUTINES IN SCHEMA public TO codehound_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO codehound_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO codehound_app;
GRANT codehound_app TO CURRENT_USER;
