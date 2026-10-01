-- =============================================================================
-- Migration 038: Partitioned Analytics Warehouse & DORA Rollups
--
-- Reference: ScanDrix Enterprise Edition TRD §3.5 & PRD REQ-6.1, REQ-6.2
--
-- Architecture
-- ------------
-- 1. analytics_pull_request_events: Range-partitioned by month on created_at.
--    Enables high-throughput append-only event logging for PR lifecycles with
--    constraint exclusion pruning for time-windowed DORA analytics queries.
--    Includes a DEFAULT partition to safely capture any events outside predefined
--    ranges without insert failures.
--
-- 2. materialized_dora_daily_rollups: Stores pre-computed daily rollups of the
--    4 canonical DORA metrics (Deployment Frequency, Lead Time for Changes,
--    Change Failure Rate, and Time to Restore Service / MTTR) per workspace.
--
-- 3. Row-Level Security: Both tables enforce strict tenant isolation via
--    app.current_tenant_id and app.is_system_worker bypass for the cron aggregator.
--    Permissions granted to scandrix_runtime for least-privilege operations.
-- =============================================================================

-- 1. Core Pull Request Event Warehouse Table (Partitioned by Month)
CREATE TABLE IF NOT EXISTS analytics_pull_request_events (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    pr_number INT NOT NULL,
    pr_title TEXT NOT NULL DEFAULT '',
    category VARCHAR(32) NOT NULL DEFAULT 'feature', -- feature, bug_fix, security, refactor
    author_email VARCHAR(255) NOT NULL DEFAULT '',
    lines_added INT NOT NULL DEFAULT 0,
    lines_deleted INT NOT NULL DEFAULT 0,
    files_changed INT NOT NULL DEFAULT 0,
    turnaround_seconds INT,
    review_status VARCHAR(32) NOT NULL DEFAULT 'PENDING', -- APPROVED, CHANGES_REQUESTED, MERGED, CLOSED
    created_at TIMESTAMPTZ NOT NULL,
    merged_at TIMESTAMPTZ,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Monthly partitions for 2026
CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_01 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_02 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-02-01 00:00:00+00') TO ('2026-03-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_03 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-03-01 00:00:00+00') TO ('2026-04-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_04 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-04-01 00:00:00+00') TO ('2026-05-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_05 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-05-01 00:00:00+00') TO ('2026-06-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_06 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-06-01 00:00:00+00') TO ('2026-07-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_07 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-07-01 00:00:00+00') TO ('2026-08-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_08 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-08-01 00:00:00+00') TO ('2026-09-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_09 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-09-01 00:00:00+00') TO ('2026-10-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_10 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-10-01 00:00:00+00') TO ('2026-11-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_11 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-11-01 00:00:00+00') TO ('2026-12-01 00:00:00+00');

CREATE TABLE IF NOT EXISTS analytics_pr_events_2026_12 PARTITION OF analytics_pull_request_events
    FOR VALUES FROM ('2026-12-01 00:00:00+00') TO ('2027-01-01 00:00:00+00');

-- Default partition for fail-safe ingestion
CREATE TABLE IF NOT EXISTS analytics_pr_events_default PARTITION OF analytics_pull_request_events DEFAULT;

-- Partitioned Table Indexes
CREATE INDEX IF NOT EXISTS idx_analytics_pr_workspace_created
    ON analytics_pull_request_events (workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_analytics_pr_repo_created
    ON analytics_pull_request_events (repository_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_analytics_pr_category
    ON analytics_pull_request_events (category);

-- 2. Materialized DORA Daily Rollups Table
CREATE TABLE IF NOT EXISTS materialized_dora_daily_rollups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    rollup_date DATE NOT NULL,
    deployment_frequency FLOAT8,          -- Deploys / merged PRs per day
    lead_time_seconds FLOAT8,             -- P50 seconds from commit/open to merge
    lead_time_p90_seconds FLOAT8,         -- P90 seconds from commit/open to merge
    change_failure_rate FLOAT8,           -- Ratio of failed/hotfix deployments (0.0 - 1.0)
    mttr_seconds FLOAT8,                  -- Mean time to restore / remediate defects
    total_reviews INT NOT NULL DEFAULT 0,
    total_findings INT NOT NULL DEFAULT 0,
    clean_reviews_count INT NOT NULL DEFAULT 0,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint ensuring single rollup per tenant/repo/day
CREATE UNIQUE INDEX IF NOT EXISTS uq_dora_rollup_workspace_repo_date
    ON materialized_dora_daily_rollups (workspace_id, COALESCE(repository_id, '00000000-0000-0000-0000-000000000000'::uuid), rollup_date);

CREATE INDEX IF NOT EXISTS idx_dora_daily_rollups_date
    ON materialized_dora_daily_rollups (workspace_id, rollup_date DESC);

-- 3. Row-Level Security
ALTER TABLE analytics_pull_request_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE analytics_pull_request_events FORCE ROW LEVEL SECURITY;

ALTER TABLE materialized_dora_daily_rollups ENABLE ROW LEVEL SECURITY;
ALTER TABLE materialized_dora_daily_rollups FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_analytics_pr_events ON analytics_pull_request_events;
CREATE POLICY tenant_isolation_analytics_pr_events ON analytics_pull_request_events
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_dora_rollups ON materialized_dora_daily_rollups;
CREATE POLICY tenant_isolation_dora_rollups ON materialized_dora_daily_rollups
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );

-- 4. Grant runtime permissions for least-privilege operations
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_runtime') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON analytics_pull_request_events TO scandrix_runtime;
        GRANT SELECT, INSERT, UPDATE, DELETE ON materialized_dora_daily_rollups TO scandrix_runtime;
    END IF;
END
$$;
