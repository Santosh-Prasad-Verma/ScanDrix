package migrations

import (
	"context"
)

// Migration027GlobalParameters represents database migration 027_global_parameters.
type Migration027GlobalParameters struct{}

// Version returns the unique migration version string.
func (m *Migration027GlobalParameters) Version() string {
	return "027"
}

// Name returns the descriptive name of the migration.
func (m *Migration027GlobalParameters) Name() string {
	return "027_global_parameters"
}

// Up applies the schema changes defined in 027_global_parameters.sql.
func (m *Migration027GlobalParameters) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 027: Global Parameters
-- System-wide parameters (telemetry_state, drixy_fine_tuning_config, max_files, etc.)

CREATE TABLE IF NOT EXISTS global_parameters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key VARCHAR(64) NOT NULL UNIQUE,
    config_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Point lookup index on config_key
CREATE INDEX IF NOT EXISTS idx_global_params_config_key 
    ON global_parameters (config_key);

-- GIN index on config_value for JSON querying
CREATE INDEX IF NOT EXISTS idx_global_params_config_value_gin 
    ON global_parameters USING GIN (config_value);

-- Row-Level Security: Global parameters are system-wide metadata.
-- Allow system background workers and authenticated platform operators.
ALTER TABLE global_parameters ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS system_worker_global_parameters ON global_parameters;
CREATE POLICY system_worker_global_parameters ON global_parameters
    FOR ALL USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR current_setting('app.current_user_role', true) IN ('owner', 'admin', 'system')
        OR current_user IN ('postgres', 'supabase_admin', 'service_role')
    );`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 027_global_parameters.sql.
func (m *Migration027GlobalParameters) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS global_parameters CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
