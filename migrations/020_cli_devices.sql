-- Migration 020: CLI Hardware Devices Table for Device Quota & Tracking
-- ScanDrix enterprise hardware device management (cli_devices)

CREATE TABLE IF NOT EXISTS cli_devices (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    device_id VARCHAR(255) NOT NULL,
    device_token_hash VARCHAR(255) NOT NULL,
    user_agent TEXT,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_cli_device_workspace UNIQUE (workspace_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_cli_devices_ws ON cli_devices(workspace_id);
CREATE INDEX IF NOT EXISTS idx_cli_devices_dev ON cli_devices(device_id);

ALTER TABLE cli_devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE cli_devices FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_cli_devices ON cli_devices;
CREATE POLICY tenant_isolation_cli_devices ON cli_devices
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
        OR workspace_id = NULLIF(current_setting('app.current_workspace_id', true), '')::uuid
    );


-- Skipped when the least-privilege runtime role has not been provisioned (CI,
-- or provisioned out-of-band): granting to a non-existent role raises 42704 and
-- rolls the whole migration back. With no runtime role there is nothing to
-- grant to. See migrations/ops/002_least_privilege_runtime_role.sql.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_app') THEN
        RAISE NOTICE 'Skipping grants to scandrix_app: the role does not exist.';
        RETURN;
    END IF;
    EXECUTE 'GRANT SELECT, INSERT, UPDATE, DELETE ON cli_devices TO scandrix_app;';
END
$$;
