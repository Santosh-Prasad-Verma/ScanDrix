-- Migration 011: Team CLI Keys Table for CLI API Authentication
-- ScanDrix team key lifecycle and verification

CREATE TABLE IF NOT EXISTS team_cli_key (
    uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    "keyHash" VARCHAR(255) NOT NULL,
    "keyPrefix" VARCHAR(32) NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT true,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    "lastUsedAt" TIMESTAMPTZ,
    "expiresAt" TIMESTAMPTZ,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_team_cli_key_prefix ON team_cli_key("keyPrefix");
CREATE INDEX IF NOT EXISTS idx_team_cli_key_hash ON team_cli_key("keyHash");
CREATE INDEX IF NOT EXISTS idx_team_cli_key_ws ON team_cli_key(workspace_id);

ALTER TABLE team_cli_key ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_cli_key FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_team_cli_key ON team_cli_key;
CREATE POLICY tenant_isolation_team_cli_key ON team_cli_key
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

