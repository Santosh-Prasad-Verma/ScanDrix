package migrations

import (
	"context"
)

// Migration023OrganizationParameters represents database migration 023_organization_parameters.
type Migration023OrganizationParameters struct{}

// Version returns the unique migration version string.
func (m *Migration023OrganizationParameters) Version() string {
	return "023"
}

// Name returns the descriptive name of the migration.
func (m *Migration023OrganizationParameters) Name() string {
	return "023_organization_parameters"
}

// Up applies the schema changes defined in 023_organization_parameters.sql.
func (m *Migration023OrganizationParameters) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 023: ScanDrix organization_parameters table
-- Supports BYOK config v2, review presets, metrics visibility, fine-tuning.

CREATE TABLE IF NOT EXISTS organization_parameters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    config_key VARCHAR(64) NOT NULL,
    config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_org_params_ws_key UNIQUE (workspace_id, config_key)
);

-- Index on (workspace_id, config_key, is_active) for ultra-fast point lookup
CREATE INDEX IF NOT EXISTS idx_org_params_ws_key_active 
    ON organization_parameters (workspace_id, config_key, is_active);

-- GIN index on config_value for deep JSON querying across credentials, models, and routing
CREATE INDEX IF NOT EXISTS idx_org_params_config_value_gin 
    ON organization_parameters USING GIN (config_value);

-- Row-Level Security (RLS) Multi-Tenant Isolation
ALTER TABLE organization_parameters ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_org_parameters ON organization_parameters;
CREATE POLICY tenant_isolation_org_parameters ON organization_parameters
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 023_organization_parameters.sql.
func (m *Migration023OrganizationParameters) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS organization_parameters CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
