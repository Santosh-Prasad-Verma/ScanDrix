-- Migration 018: Row-Level Security on Root Workspaces Table
-- Enforces strict multi-tenant boundaries on the root workspaces table
-- to prevent cross-tenant data leakage while permitting system workers to operate.

-- Enable and FORCE Row-Level Security on workspaces table
ALTER TABLE workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE workspaces FORCE ROW LEVEL SECURITY;

-- Create tenant isolation policy for workspaces
DROP POLICY IF EXISTS tenant_isolation_workspaces ON workspaces;
CREATE POLICY tenant_isolation_workspaces ON workspaces
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
