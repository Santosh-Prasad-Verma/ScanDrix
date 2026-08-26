-- ============================================================================
-- CodeHound Migration 000001 Rollback: Drop Initial Core Schema
-- ============================================================================

-- 1. Drop RLS Policies
DROP POLICY IF EXISTS tenant_isolation_findings ON findings;
DROP POLICY IF EXISTS tenant_isolation_scans ON scans;
DROP POLICY IF EXISTS tenant_isolation_projects ON projects;
DROP POLICY IF EXISTS tenant_isolation_api_keys ON api_keys;
DROP POLICY IF EXISTS tenant_isolation_policy_profiles ON policy_profiles;

-- 2. Drop Tables (in reverse topological dependency order)
DROP TABLE IF EXISTS audit_events_y2026m10 CASCADE;
DROP TABLE IF EXISTS audit_events_y2026m09 CASCADE;
DROP TABLE IF EXISTS audit_events_y2026m08 CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;

DROP TABLE IF EXISTS idempotency_keys CASCADE;
DROP TABLE IF EXISTS artifacts CASCADE;

DROP TABLE IF EXISTS model_runs_y2026m10 CASCADE;
DROP TABLE IF EXISTS model_runs_y2026m09 CASCADE;
DROP TABLE IF EXISTS model_runs_y2026m08 CASCADE;
DROP TABLE IF EXISTS model_runs CASCADE;

DROP TABLE IF EXISTS test_runs CASCADE;

DROP TABLE IF EXISTS executions_y2026m10 CASCADE;
DROP TABLE IF EXISTS executions_y2026m09 CASCADE;
DROP TABLE IF EXISTS executions_y2026m08 CASCADE;
DROP TABLE IF EXISTS executions CASCADE;

DROP TABLE IF EXISTS patches CASCADE;
DROP TABLE IF EXISTS evidence CASCADE;

DROP TABLE IF EXISTS finding_occurrences_y2026m10 CASCADE;
DROP TABLE IF EXISTS finding_occurrences_y2026m09 CASCADE;
DROP TABLE IF EXISTS finding_occurrences_y2026m08 CASCADE;
DROP TABLE IF EXISTS finding_occurrences CASCADE;

DROP TABLE IF EXISTS findings CASCADE;
DROP TABLE IF EXISTS scans CASCADE;
DROP TABLE IF EXISTS test_targets CASCADE;
DROP TABLE IF EXISTS authorization_grants CASCADE;
DROP TABLE IF EXISTS code_edges CASCADE;
DROP TABLE IF EXISTS code_symbols CASCADE;
DROP TABLE IF EXISTS code_files CASCADE;
DROP TABLE IF EXISTS commits CASCADE;
DROP TABLE IF EXISTS repositories CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS policy_profiles CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS tenants CASCADE;

-- 3. Drop Custom Types & Enums
DROP TYPE IF EXISTS patch_status_enum;
DROP TYPE IF EXISTS execution_status_enum;
DROP TYPE IF EXISTS sandbox_tier_enum;
DROP TYPE IF EXISTS execution_type_enum;
DROP TYPE IF EXISTS finding_category_enum;
DROP TYPE IF EXISTS finding_state_enum;
DROP TYPE IF EXISTS finding_severity_enum;
DROP TYPE IF EXISTS scan_status_enum;
DROP TYPE IF EXISTS scan_type_enum;
DROP TYPE IF EXISTS tenant_status_enum;
DROP TYPE IF EXISTS tenant_plan_enum;

-- 4. Drop Application Roles
DROP ROLE IF EXISTS codehound_app;
