-- Migration 008: RLS Hardening, Embedding Column, and Missing Tenant Isolation
-- Fixes:
-- 1. FORCE ROW LEVEL SECURITY on migration-004 tables (warehouse_domain_events,
--    organization_billing_seats, drixy_embedding_vectors) so the table owner
--    cannot bypass RLS policies.
-- 2. Add actual vector storage column to drixy_embedding_vectors.
-- 3. Add RLS to code_ast_nodes and code_ast_edges (migration 003) which had
--    no tenant isolation at all.
-- 4. Add RLS to team_members (migration 005) which had no tenant isolation.

-- =========================================================================
-- 1. FORCE ROW LEVEL SECURITY on Migration-004 Tables
-- ENABLE ROW LEVEL SECURITY alone does NOT restrict the table owner. Without
-- FORCE, the owner role (typically the migration user) bypasses all policies.
-- =========================================================================

ALTER TABLE warehouse_domain_events FORCE ROW LEVEL SECURITY;
ALTER TABLE organization_billing_seats FORCE ROW LEVEL SECURITY;
ALTER TABLE drixy_embedding_vectors FORCE ROW LEVEL SECURITY;


-- =========================================================================
-- 2. Add embedding vector storage to drixy_embedding_vectors
-- The table had embedding_dim and embedding_model metadata but no actual
-- vector column. We use FLOAT8[] as a portable fallback; if pgvector is
-- installed, a typed vector column can be added separately.
-- =========================================================================

ALTER TABLE drixy_embedding_vectors
    ADD COLUMN IF NOT EXISTS embedding FLOAT8[];

COMMENT ON COLUMN drixy_embedding_vectors.embedding IS
    'Dense float64 vector. Length MUST match embedding_dim. Use pgvector vector() type for indexed ANN search.';


-- =========================================================================
-- 3. Add RLS to code_ast_nodes and code_ast_edges (Migration 003)
-- These tables reference repository_id, not workspace_id directly. The RLS
-- policy joins through tracked_repositories to enforce workspace isolation.
-- =========================================================================

-- 3a. code_ast_nodes
ALTER TABLE code_ast_nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_ast_nodes FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_code_ast_nodes ON code_ast_nodes;
CREATE POLICY tenant_isolation_code_ast_nodes ON code_ast_nodes
    FOR ALL
    USING (
        repository_id IN (
            SELECT id FROM tracked_repositories
            WHERE workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        )
    );

-- 3b. code_ast_edges
ALTER TABLE code_ast_edges ENABLE ROW LEVEL SECURITY;
ALTER TABLE code_ast_edges FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_code_ast_edges ON code_ast_edges;
CREATE POLICY tenant_isolation_code_ast_edges ON code_ast_edges
    FOR ALL
    USING (
        repository_id IN (
            SELECT id FROM tracked_repositories
            WHERE workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        )
    );


-- =========================================================================
-- 4. Add RLS to team_members (Migration 005)
-- team_members references team_id → teams.workspace_id for isolation.
-- =========================================================================

ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_team_members ON team_members;
CREATE POLICY tenant_isolation_team_members ON team_members
    FOR ALL
    USING (
        team_id IN (
            SELECT id FROM teams
            WHERE workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        )
    );


-- =========================================================================
-- 5. Also FORCE RLS on all existing migration-005 tables for consistency
-- (they already had ENABLE but not FORCE)
-- =========================================================================

ALTER TABLE teams FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace_parameters FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_channels FORCE ROW LEVEL SECURITY;
ALTER TABLE integration_connections FORCE ROW LEVEL SECURITY;
ALTER TABLE finding_feedback FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
ALTER TABLE token_usage_records FORCE ROW LEVEL SECURITY;
ALTER TABLE workspace_spend_limits FORCE ROW LEVEL SECURITY;
ALTER TABLE organization_licenses FORCE ROW LEVEL SECURITY;

-- =========================================================================
-- 6. Add last_active_at column to account_profiles for seat reclamation
-- =========================================================================

ALTER TABLE account_profiles
    ADD COLUMN IF NOT EXISTS last_active_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

