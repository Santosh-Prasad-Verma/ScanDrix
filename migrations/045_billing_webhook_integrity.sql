-- Provider order identity is global. Refuse duplicate historical identities
-- rather than silently choosing an owner or deleting financial records.
CREATE UNIQUE INDEX IF NOT EXISTS billing_transactions_provider_order_unique
    ON billing_transactions(provider, order_id);

ALTER TABLE billing_transactions FORCE ROW LEVEL SECURITY;

-- Signed webhook ingress and reconciliation need a read-only reverse lookup.
-- Writes still require the existing explicit tenant policy from migration 006.
DROP POLICY IF EXISTS billing_system_lookup ON billing_transactions;
CREATE POLICY billing_system_lookup ON billing_transactions
    FOR SELECT
    USING (current_setting('app.is_system_worker', true) = 'true');
