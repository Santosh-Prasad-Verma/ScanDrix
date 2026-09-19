package migrations

import (
	"context"
)

// Migration009ReviewAttestations represents database migration 009_review_attestations.
type Migration009ReviewAttestations struct{}

// Version returns the unique migration version string.
func (m *Migration009ReviewAttestations) Version() string {
	return "009"
}

// Name returns the descriptive name of the migration.
func (m *Migration009ReviewAttestations) Name() string {
	return "009_review_attestations"
}

// Up applies the schema changes defined in 009_review_attestations.sql.
func (m *Migration009ReviewAttestations) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 009: Cryptographic Attestations, PM Finding Tickets, and Auto-Ticket Configs
-- Master Rule 4.4 & 5.3 compliant multi-tenant row-level security schema

-- 1. Review Attestations (In-Toto v1 & SLSA v1.0 DSSE Envelopes)
CREATE TABLE IF NOT EXISTS review_attestations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    review_id UUID NOT NULL REFERENCES pull_request_reviews(id) ON DELETE CASCADE,
    predicate_type VARCHAR(255) NOT NULL,
    decision VARCHAR(64) NOT NULL,
    key_id VARCHAR(255) NOT NULL,
    envelope_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, review_id, predicate_type)
);

CREATE INDEX IF NOT EXISTS idx_review_attestations_ws ON review_attestations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_review_attestations_review ON review_attestations(review_id);

ALTER TABLE review_attestations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_review_attestations ON review_attestations;
CREATE POLICY tenant_isolation_review_attestations ON review_attestations
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 2. Finding PM Tickets (Jira, Linear, Azure Boards)
CREATE TABLE IF NOT EXISTS finding_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    finding_id UUID NOT NULL REFERENCES code_findings(id) ON DELETE CASCADE,
    platform VARCHAR(64) NOT NULL,
    ticket_key VARCHAR(255) NOT NULL,
    ticket_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, finding_id, platform)
);

CREATE INDEX IF NOT EXISTS idx_finding_tickets_ws ON finding_tickets(workspace_id);
CREATE INDEX IF NOT EXISTS idx_finding_tickets_finding ON finding_tickets(finding_id);

ALTER TABLE finding_tickets ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_finding_tickets ON finding_tickets;
CREATE POLICY tenant_isolation_finding_tickets ON finding_tickets
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 3. PM Auto-Ticket Configs
CREATE TABLE IF NOT EXISTS pm_auto_ticket_configs (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    repository_id UUID NOT NULL REFERENCES tracked_repositories(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT false,
    platform VARCHAR(64) NOT NULL DEFAULT 'jira',
    project_key VARCHAR(128) NOT NULL DEFAULT '',
    issue_type VARCHAR(64) NOT NULL DEFAULT 'Bug',
    min_severity VARCHAR(32) NOT NULL DEFAULT 'HIGH',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, repository_id)
);

CREATE INDEX IF NOT EXISTS idx_pm_auto_ticket_ws_repo ON pm_auto_ticket_configs(workspace_id, repository_id);

ALTER TABLE pm_auto_ticket_configs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_pm_auto_ticket_configs ON pm_auto_ticket_configs;
CREATE POLICY tenant_isolation_pm_auto_ticket_configs ON pm_auto_ticket_configs
    FOR ALL USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);


-- 4. CLI Auth Sessions
CREATE TABLE IF NOT EXISTS cli_auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id VARCHAR(255) NOT NULL UNIQUE,
    user_id UUID,
    workspace_id UUID,
    status VARCHAR(64) NOT NULL DEFAULT 'PENDING',
    token TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_sid ON cli_auth_sessions(session_id);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 009_review_attestations.sql.
func (m *Migration009ReviewAttestations) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS cli_auth_sessions, pm_auto_ticket_configs, finding_tickets, review_attestations CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
