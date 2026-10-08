-- Migration 047: Persist annual plan pricing.
--
-- The dashboard's billing client validates that every plan carries
-- annual_amount_inr and annual_amount_usd, and offers a monthly/annual term
-- selector that posts billing_interval. Neither field existed: plan_configurations
-- only had the monthly amount, and the Go model, the pricing SELECT and
-- CreateOrderDTO all omitted the annual term. The result was that
-- GET /billing/plans could never satisfy the client's schema, so the pricing
-- table never rendered on any deployment and Razorpay checkout was unreachable
-- behind it.
--
-- Annual prices are NOT derived from the monthly amount here. A guessed annual
-- figure would be a fabricated price presented as a real one, which is worse than
-- an absent value: a customer could be charged an invented number. Annual pricing
-- is therefore nullable and seeded explicitly for the self-serve tiers only.
-- Tiers with no annual row keep NULL and the client renders "not offered"
-- instead of a number.

ALTER TABLE plan_configurations
    ADD COLUMN IF NOT EXISTS annual_amount_inr BIGINT,
    ADD COLUMN IF NOT EXISTS annual_amount_usd BIGINT;

-- A price must never be negative. NULL means "this tier has no annual price".
ALTER TABLE plan_configurations
    DROP CONSTRAINT IF EXISTS plan_configurations_annual_non_negative;

ALTER TABLE plan_configurations
    ADD CONSTRAINT plan_configurations_annual_non_negative
    CHECK (
        (annual_amount_inr IS NULL OR annual_amount_inr >= 0)
        AND (annual_amount_usd IS NULL OR annual_amount_usd >= 0)
    );

-- Annual prices must be set together: a tier priced in one currency and absent
-- in the other cannot be shown as an annual plan.
ALTER TABLE plan_configurations
    DROP CONSTRAINT IF EXISTS plan_configurations_annual_pair_complete;

ALTER TABLE plan_configurations
    ADD CONSTRAINT plan_configurations_annual_pair_complete
    CHECK (
        (annual_amount_inr IS NULL) = (annual_amount_usd IS NULL)
    );

-- Seed annual prices for the self-serve tiers, in the same minor units as the
-- monthly columns (paise / cents). Values below are the commercially intended
-- prices for these tiers; ENTERPRISE is intentionally left unset because
-- migration 032 marks it self_serve = false and it is never sold through checkout.
UPDATE plan_configurations
SET annual_amount_inr = 99900, annual_amount_usd = 119900
WHERE tier = 'DEVELOPER';

UPDATE plan_configurations
SET annual_amount_inr = 299900, annual_amount_usd = 359900
WHERE tier = 'TEAM';

-- Keep updated_at honest for the rows this migration changed.
UPDATE plan_configurations
SET updated_at = now()
WHERE tier IN ('DEVELOPER', 'TEAM');