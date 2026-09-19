-- Migration 015: Non-Superuser Application Role Provisioning & RLS Hardening
-- Creates the least-privileged runtime application role 'scandrix_app' enforcing
-- Row-Level Security (NOBYPASSRLS) and adds webhook_secret_enc columns for dynamic secret resolution.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_app') THEN
        BEGIN
            CREATE ROLE scandrix_app WITH LOGIN PASSWORD 'scandrix_secure_pass' NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE;
        EXCEPTION WHEN OTHERS THEN
            RAISE NOTICE 'Role scandrix_app could not be created or already exists: %', SQLERRM;
        END;
    END IF;
END $$;

-- Grant schema and table permissions to scandrix_app
GRANT USAGE ON SCHEMA public TO scandrix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO scandrix_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO scandrix_app;

ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO scandrix_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO scandrix_app;

-- Add encrypted webhook secret support to tracked_repositories and integration_connections
ALTER TABLE tracked_repositories ADD COLUMN IF NOT EXISTS webhook_secret_enc TEXT NOT NULL DEFAULT '';
ALTER TABLE integration_connections ADD COLUMN IF NOT EXISTS webhook_secret_enc TEXT NOT NULL DEFAULT '';

-- Enable and FORCE Row-Level Security explicitly
ALTER TABLE tracked_repositories ENABLE ROW LEVEL SECURITY;
ALTER TABLE tracked_repositories FORCE ROW LEVEL SECURITY;
ALTER TABLE integration_connections ENABLE ROW LEVEL SECURITY;
ALTER TABLE integration_connections FORCE ROW LEVEL SECURITY;

-- Update RLS policies on tracked_repositories and integration_connections
-- to allow system worker processes (app.is_system_worker = 'true') to resolve records across tenants
DROP POLICY IF EXISTS tenant_isolation_repos ON tracked_repositories;
CREATE POLICY tenant_isolation_repos ON tracked_repositories
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_integrations ON integration_connections;
CREATE POLICY tenant_isolation_integrations ON integration_connections
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
