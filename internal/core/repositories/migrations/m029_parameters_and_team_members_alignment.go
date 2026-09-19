package migrations

import (
	"context"
)

// Migration029ParametersAndTeamMembersAlignment represents database migration 029_parameters_and_team_members_alignment.
type Migration029ParametersAndTeamMembersAlignment struct{}

// Version returns the unique migration version string.
func (m *Migration029ParametersAndTeamMembersAlignment) Version() string {
	return "029"
}

// Name returns the descriptive name of the migration.
func (m *Migration029ParametersAndTeamMembersAlignment) Name() string {
	return "029_parameters_and_team_members_alignment"
}

// Up applies the schema changes defined in 029_parameters_and_team_members_alignment.sql.
func (m *Migration029ParametersAndTeamMembersAlignment) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 029: Parameters Table & Team Member Profile Alignment
-- Matches ScanDrix AI ParametersModel & TeamMemberModel architecture.

-- 1. Create parameters table for team-scoped and workspace-scoped configurations
CREATE TABLE IF NOT EXISTS parameters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    config_key VARCHAR(64) NOT NULL,
    config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT true,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Unique constraint: exactly one active parameter per (workspace, team, config_key)
CREATE UNIQUE INDEX IF NOT EXISTS uq_parameters_ws_team_key 
    ON parameters (workspace_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid), config_key) 
    WHERE active = true;

-- Performance index for point lookups by key and team
CREATE INDEX IF NOT EXISTS idx_parameters_lookup 
    ON parameters (workspace_id, team_id, config_key, active);

-- GIN index for JSON queries into review thresholds and configs
CREATE INDEX IF NOT EXISTS idx_parameters_config_value_gin 
    ON parameters USING GIN (config_value);

-- Row-Level Security (RLS) Tenant Isolation
ALTER TABLE parameters ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_parameters ON parameters;
CREATE POLICY tenant_isolation_parameters ON parameters
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 2. Enhance team_members table for full profile alignment
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS name VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS status BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS code_management JSONB;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS communication JSONB;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS project_management JSONB;
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS communication_id VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE team_members ADD COLUMN IF NOT EXISTS review_count INT NOT NULL DEFAULT 0;

-- Backfill name from email where empty
UPDATE team_members SET name = split_part(email, '@', 1) WHERE name = '' AND email <> '';

-- 3. Enhance cli_devices table with user reference
ALTER TABLE cli_devices ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(uuid) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_cli_devices_user ON cli_devices(user_id) WHERE user_id IS NOT NULL;`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 029_parameters_and_team_members_alignment.sql.
func (m *Migration029ParametersAndTeamMembersAlignment) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS parameters CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
