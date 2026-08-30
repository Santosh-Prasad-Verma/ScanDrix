-- Migration 005: Teams, Parameters, Notification Channels, Integrations, Audit Logs, Token Usage & Licenses
-- Master Rule 4.4 & 5.3 compliant schema with multi-tenant row-level security

-- 1. Teams & Team Members
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure all required columns exist in case the table was created by a previous schema
DO $$
BEGIN
    ALTER TABLE teams ADD COLUMN IF NOT EXISTS id UUID DEFAULT gen_random_uuid();
    ALTER TABLE teams ADD COLUMN IF NOT EXISTS workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE;
    ALTER TABLE teams ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
    ALTER TABLE teams ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
    ALTER TABLE teams ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
    
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'teams' AND column_name = 'uuid') THEN
        UPDATE teams SET id = uuid WHERE id IS NULL;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_id ON teams(id);
CREATE INDEX IF NOT EXISTS idx_teams_ws ON teams(workspace_id);
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_teams ON teams;
CREATE POLICY tenant_isolation_teams ON teams
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE TABLE IF NOT EXISTS team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    email VARCHAR(255) NOT NULL DEFAULT '',
    role VARCHAR(32) NOT NULL DEFAULT 'MEMBER',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);

-- 2. Workspace Parameters & Review Thresholds
CREATE TABLE IF NOT EXISTS workspace_parameters (
    workspace_id UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    review_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    org_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE workspace_parameters ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_workspace_params ON workspace_parameters;
CREATE POLICY tenant_isolation_workspace_params ON workspace_parameters
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 3. Notification Channels
CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL,
    target TEXT NOT NULL,
    severity VARCHAR(32) NOT NULL DEFAULT 'HIGH',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notif_channels_ws ON notification_channels(workspace_id);
ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_notif_channels ON notification_channels;
CREATE POLICY tenant_isolation_notif_channels ON notification_channels
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 4. Integration Connections (GitHub, GitLab, Bitbucket, Azure DevOps)
CREATE TABLE IF NOT EXISTS integration_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    is_connected BOOLEAN NOT NULL DEFAULT true,
    access_token_enc TEXT NOT NULL DEFAULT '',
    repo_count INT NOT NULL DEFAULT 0,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, provider)
);

CREATE INDEX IF NOT EXISTS idx_integrations_ws ON integration_connections(workspace_id);
ALTER TABLE integration_connections ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_integrations ON integration_connections;
CREATE POLICY tenant_isolation_integrations ON integration_connections
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 5. Finding Feedback (Tuning Accuracy)
CREATE TABLE IF NOT EXISTS finding_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    finding_id UUID NOT NULL REFERENCES code_findings(id) ON DELETE CASCADE,
    sentiment VARCHAR(32) NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feedback_finding ON finding_feedback(finding_id);
ALTER TABLE finding_feedback ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_feedback ON finding_feedback;
CREATE POLICY tenant_isolation_feedback ON finding_feedback
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 6. Audit Logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_id VARCHAR(128) NOT NULL DEFAULT '',
    actor_email VARCHAR(255) NOT NULL DEFAULT '',
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    action VARCHAR(128) NOT NULL,
    target_type VARCHAR(64) NOT NULL DEFAULT '',
    target_id VARCHAR(128) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_ws_action ON audit_logs(workspace_id, action, created_at);
ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_audit_logs ON audit_logs;
CREATE POLICY tenant_isolation_audit_logs ON audit_logs
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 7. Token Usage & Spend Limits
CREATE TABLE IF NOT EXISTS token_usage_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    review_id UUID REFERENCES pull_request_reviews(id) ON DELETE SET NULL,
    prompt_tokens BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cost_usd NUMERIC(10, 4) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_token_usage_ws_created ON token_usage_records(workspace_id, created_at);
ALTER TABLE token_usage_records ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_token_usage ON token_usage_records;
CREATE POLICY tenant_isolation_token_usage ON token_usage_records
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

CREATE TABLE IF NOT EXISTS workspace_spend_limits (
    workspace_id UUID PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    monthly_spend_limit_usd NUMERIC(10, 2) NOT NULL DEFAULT 50.00,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE workspace_spend_limits ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_spend_limits ON workspace_spend_limits;
CREATE POLICY tenant_isolation_spend_limits ON workspace_spend_limits
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 8. Organization Licenses
CREATE TABLE IF NOT EXISTS organization_licenses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    license_key VARCHAR(255) NOT NULL UNIQUE,
    organization_name VARCHAR(255) NOT NULL DEFAULT '',
    plan_tier VARCHAR(64) NOT NULL DEFAULT 'ENTERPRISE',
    total_seats INT NOT NULL DEFAULT 100,
    allocated_seats INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    is_air_gapped BOOLEAN NOT NULL DEFAULT false,
    features_enabled JSONB NOT NULL DEFAULT '[]'::jsonb,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_licenses_ws ON organization_licenses(workspace_id);
ALTER TABLE organization_licenses ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_licenses ON organization_licenses;
CREATE POLICY tenant_isolation_licenses ON organization_licenses
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);
