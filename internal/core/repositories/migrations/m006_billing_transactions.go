package migrations

import (
	"context"
)

// Migration006BillingTransactions represents database migration 006_billing_transactions.
type Migration006BillingTransactions struct{}

// Version returns the unique migration version string.
func (m *Migration006BillingTransactions) Version() string {
	return "006"
}

// Name returns the descriptive name of the migration.
func (m *Migration006BillingTransactions) Name() string {
	return "006_billing_transactions"
}

// Up applies the schema changes defined in 006_billing_transactions.sql.
func (m *Migration006BillingTransactions) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 006: Razorpay Billing Transactions & Plan State
-- Compliant with Master Rules 4.4 & 5.3 (Row-Level Tenant Isolation)

CREATE TABLE IF NOT EXISTS billing_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider VARCHAR(64) NOT NULL DEFAULT 'razorpay',
    order_id VARCHAR(128) NOT NULL,
    payment_id VARCHAR(128),
    signature VARCHAR(256),
    amount BIGINT NOT NULL,
    currency VARCHAR(16) NOT NULL DEFAULT 'INR',
    plan_tier VARCHAR(64) NOT NULL DEFAULT 'TEAM',
    status VARCHAR(64) NOT NULL DEFAULT 'created',
    receipt VARCHAR(128),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_billing_tx_ws ON billing_transactions(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_billing_tx_order ON billing_transactions(order_id);
CREATE INDEX IF NOT EXISTS idx_billing_tx_payment ON billing_transactions(payment_id);

-- Enable Row Level Security
ALTER TABLE billing_transactions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_billing_tx ON billing_transactions;
CREATE POLICY tenant_isolation_billing_tx ON billing_transactions
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 006_billing_transactions.sql.
func (m *Migration006BillingTransactions) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS billing_transactions CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
