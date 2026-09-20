-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- ScanDrix global_parameters table.
-- System-wide parameters (telemetry_state, drixy_fine_tuning_config, max_files, etc.)
-- ═══════════════════════════════════════════════════════════════

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
    );
