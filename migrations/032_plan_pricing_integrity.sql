-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- Migration 032: Plan pricing integrity — 4-tier lineup + explicit display order
--
-- Corrections to the plan_configurations seed:
--
-- 1. ScanDrix operates exactly 4 plan tiers:
--    - COMMUNITY (Free / Open Source)
--    - DEVELOPER (Individual Maintainers / Small Projects)
--    - TEAM (Growing Teams shipping to production)
--    - ENTERPRISE (Custom Quoted / VPC / Air-Gapped / Dedicated SLA)
--
--    The old SCALE tier was withdrawn as it duplicated Enterprise capabilities
--    at a self-serve price. This migration ensures any lingering SCALE rows
--    are purged and the 4 official tiers are consistently ordered.
--
-- 2. Explicit sort_order:
--    ListPlanConfigurations previously ordered by `amount_inr ASC`. Deriving
--    presentation order from price meant discounts, promotions, or enterprise
--    custom quotes silently reordered the pricing table. Display order is now
--    an explicit ascending sequence (10, 20, 30, 40).
--
-- 3. Self-serve flag:
--    COMMUNITY, DEVELOPER, and TEAM are self-serve checkout plans. ENTERPRISE
--    is custom quoted (self_serve = false).
--
-- Deliberately does NOT edit migration 007 (which already executed on live DBs).
-- 032 is additive and idempotent.

-- ---------------------------------------------------------------------------
-- 1. Explicit presentation order + self-serve flag for the 4 official tiers:
--    COMMUNITY (10), DEVELOPER (20), TEAM (30), ENTERPRISE (40)
-- ---------------------------------------------------------------------------

ALTER TABLE plan_configurations
    ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

ALTER TABLE plan_configurations
    ADD COLUMN IF NOT EXISTS self_serve BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN plan_configurations.sort_order IS
    'Ascending display order for public pricing surfaces. Must not be derived from amount_inr.';

COMMENT ON COLUMN plan_configurations.self_serve IS
    'False when the tier cannot be purchased through checkout and must be quoted (Enterprise).';

-- Remove any legacy / withdrawn SCALE plan rows (ScanDrix offers exactly 4 tiers)
DELETE FROM plan_configurations WHERE UPPER(tier) = 'SCALE';

-- Ensure DEVELOPER tier is present in plan_configurations if missing
INSERT INTO plan_configurations (
    tier, display_name, amount_inr, amount_usd, monthly_tokens, burst_limit_per_min,
    max_seats, max_repositories, max_concurrent_reviews, byok_allowed,
    sort_order, self_serve, allocated_models, features_enabled
) VALUES (
    'DEVELOPER',
    'Developer Plan',
    79900,
    999,
    6000000,
    300000,
    10,
    0,
    5,
    true,
    20,
    true,
    '["gpt-5.6-luna", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "qwen3.8-max", "kimi-k3", "deepseek-chat", "mistral-large-3", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite", "glm-5.3-flash", "minimax/minimax-m3:free"]'::jsonb,
    '["automated_reviews", "custom_rules", "unlimited_repos", "byok_encryption", "dora_metrics"]'::jsonb
)
ON CONFLICT (tier) DO UPDATE SET
    sort_order = 20,
    self_serve = true,
    updated_at = NOW();

-- Ascending display order for the 4 official tiers. Idempotent: re-running
-- recomputes the same values from the tier name.
UPDATE plan_configurations SET
    sort_order = CASE UPPER(tier)
        WHEN 'COMMUNITY'  THEN 10
        WHEN 'DEVELOPER'  THEN 20
        WHEN 'TEAM'       THEN 30
        WHEN 'ENTERPRISE' THEN 40
        ELSE 99
    END,
    updated_at = NOW()
WHERE sort_order IS DISTINCT FROM CASE UPPER(tier)
        WHEN 'COMMUNITY'  THEN 10
        WHEN 'DEVELOPER'  THEN 20
        WHEN 'TEAM'       THEN 30
        WHEN 'ENTERPRISE' THEN 40
        ELSE 99
    END;

-- Enterprise is a quoted deployment (VPC / air-gapped), not a cart button.
UPDATE plan_configurations
SET self_serve = false, updated_at = NOW()
WHERE UPPER(tier) = 'ENTERPRISE'
  AND self_serve IS DISTINCT FROM false;

-- Self-serve tiers: Community, Developer, and Team
UPDATE plan_configurations
SET self_serve = true, updated_at = NOW()
WHERE UPPER(tier) IN ('COMMUNITY', 'DEVELOPER', 'TEAM')
  AND self_serve IS DISTINCT FROM true;

-- ---------------------------------------------------------------------------
-- 3. Data-integrity guards
-- ---------------------------------------------------------------------------
-- Reject a future seed that reintroduces a negative amount or a bad sort
-- position. Without these, the inverted ladder is only caught by a human
-- noticing the public pricing page.

ALTER TABLE plan_configurations
    DROP CONSTRAINT IF EXISTS plan_configurations_amount_positive;

ALTER TABLE plan_configurations
    ADD CONSTRAINT plan_configurations_amount_positive
    CHECK (amount_inr >= 0 AND amount_usd >= 0);

ALTER TABLE plan_configurations
    DROP CONSTRAINT IF EXISTS plan_configurations_sort_order_positive;

ALTER TABLE plan_configurations
    ADD CONSTRAINT plan_configurations_sort_order_positive
    CHECK (sort_order >= 0);
