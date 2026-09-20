package migrations

import (
	"context"
)

// Migration004ExtendedWarehouseAndBilling represents database migration 004_extended_warehouse_and_billing.
type Migration004ExtendedWarehouseAndBilling struct{}

// Version returns the unique migration version string.
func (m *Migration004ExtendedWarehouseAndBilling) Version() string {
	return "004"
}

// Name returns the descriptive name of the migration.
func (m *Migration004ExtendedWarehouseAndBilling) Name() string {
	return "004_extended_warehouse_and_billing"
}

// Up applies the schema changes defined in 004_extended_warehouse_and_billing.sql.
func (m *Migration004ExtendedWarehouseAndBilling) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 004: Extended Analytics Warehouse, Seat Allocations & Drixy Vector Memory
-- Master Rule 4.4 & 5.3 compliant schema with multi-tenant row-level security

-- 1. Persistent Domain Events Warehouse
CREATE TABLE IF NOT EXISTS warehouse_domain_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(128) NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_warehouse_events_ws_type ON warehouse_domain_events(workspace_id, event_type, occurred_at);
CREATE INDEX IF NOT EXISTS idx_warehouse_events_agg ON warehouse_domain_events(aggregate_id, occurred_at);

-- Row-Level Security for Warehouse Events
ALTER TABLE warehouse_domain_events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_warehouse_events ON warehouse_domain_events;
CREATE POLICY tenant_isolation_warehouse_events ON warehouse_domain_events
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 2. Organization Billing & Seat Allocations
CREATE TABLE IF NOT EXISTS organization_billing_seats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    tier VARCHAR(64) NOT NULL DEFAULT 'standard',
    max_seats INT NOT NULL DEFAULT 10,
    allocated_seats INT NOT NULL DEFAULT 0,
    byok_enabled BOOLEAN NOT NULL DEFAULT false,
    dora_enabled BOOLEAN NOT NULL DEFAULT false,
    active_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE organization_billing_seats ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_billing_seats ON organization_billing_seats;
CREATE POLICY tenant_isolation_billing_seats ON organization_billing_seats
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);

-- 3. Drixy Vector Embeddings Memory for Rules & Suggestion Fine-Tuning
CREATE TABLE IF NOT EXISTS drixy_embedding_vectors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    rule_id UUID,
    entity_type VARCHAR(64) NOT NULL, -- 'RULE', 'SUGGESTION', 'DIFF_PATTERN'
    embedding_model VARCHAR(64) NOT NULL DEFAULT 'text-embedding-3-small',
    embedding_dim INT NOT NULL DEFAULT 1536,
    content_hash VARCHAR(64) NOT NULL,
    raw_content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_ws ON drixy_embedding_vectors(workspace_id, entity_type);
CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_hash ON drixy_embedding_vectors(content_hash);

ALTER TABLE drixy_embedding_vectors ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_embeddings ON drixy_embedding_vectors;
CREATE POLICY tenant_isolation_embeddings ON drixy_embedding_vectors
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 004_extended_warehouse_and_billing.sql.
func (m *Migration004ExtendedWarehouseAndBilling) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS warehouse_domain_events, organization_billing_seats CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
