-- Migration 015: Non-Superuser Application Role Provisioning & RLS Hardening
-- Creates the least-privileged runtime application role 'scandrix_app' enforcing
-- Row-Level Security (NOBYPASSRLS) and adds webhook_secret_enc columns for dynamic secret resolution.

-- SECURITY (AUDIT_REMEDIATION.md F-06): this migration previously created the
-- role with a hardcoded password committed to the repository. Because the
-- migration runner is unprivileged, role creation usually fails anyway; the
-- role is provisioned out-of-band by ops/002_least_privilege_runtime_role.sql.
--
-- The password is now read from current_setting('scandrix.app_password') when
-- the operator supplies it at apply time, e.g.
--
--   psql -v ON_ERROR_STOP=1 -c "SET scandrix.app_password = '...';" -f ...
--
-- or via the SCANDRIX_APP_PASSWORD environment read by scripts/migrate.sh.
-- No credential is stored in this file or anywhere else in the repository.
DO $$
DECLARE
    app_password text := current_setting('scandrix.app_password', true);
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_app') THEN
        IF app_password IS NULL OR app_password = '' THEN
            RAISE NOTICE 'Role scandrix_app not created: no password supplied. Set scandrix.app_password or provision the role out-of-band (see migrations/ops/002_least_privilege_runtime_role.sql).';
            RETURN;
        END IF;
        BEGIN
            EXECUTE format(
                'CREATE ROLE scandrix_app WITH LOGIN PASSWORD %L NOBYPASSRLS NOSUPERUSER NOCREATEDB NOCREATEROLE',
                app_password
            );
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
