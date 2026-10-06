-- Migration 043: Persist the subscription term used by Razorpay checkout.
-- Existing transactions remain monthly for backwards compatibility.

ALTER TABLE billing_transactions
    ADD COLUMN IF NOT EXISTS billing_interval VARCHAR(16) NOT NULL DEFAULT 'monthly';

UPDATE billing_transactions
SET billing_interval = 'monthly'
WHERE billing_interval IS NULL OR billing_interval = '';

ALTER TABLE billing_transactions
    DROP CONSTRAINT IF EXISTS billing_transactions_interval_valid;

ALTER TABLE billing_transactions
    ADD CONSTRAINT billing_transactions_interval_valid
    CHECK (billing_interval IN ('monthly', 'annual'));
