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

GRANT SELECT, INSERT, UPDATE, DELETE ON cli_devices TO scandrix_app;
