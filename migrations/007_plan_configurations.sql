-- Migration 007: Database-Driven Plan Configurations & Dynamic Tier Entitlements
-- Eliminates hardcoded plan pricing, quotas, and model allocation lists.

CREATE TABLE IF NOT EXISTS plan_configurations (
    tier VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(128) NOT NULL,
    amount_inr BIGINT NOT NULL,
    amount_usd BIGINT NOT NULL,
    monthly_tokens BIGINT NOT NULL,
    burst_limit_per_min BIGINT NOT NULL,
    max_seats INT NOT NULL,
    max_repositories INT NOT NULL,
    max_concurrent_reviews INT NOT NULL,
    byok_allowed BOOLEAN NOT NULL DEFAULT true,
    allocated_models JSONB NOT NULL DEFAULT '[]'::jsonb,
    features_enabled JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed defaults so there are ZERO hardcoded values in Go source code
INSERT INTO plan_configurations (
    tier, display_name, amount_inr, amount_usd, monthly_tokens, burst_limit_per_min,
    max_seats, max_repositories, max_concurrent_reviews, byok_allowed, allocated_models, features_enabled
) VALUES 
(
    'COMMUNITY',
    'Community Free Plan',
    0,
    0,
    1000000,
    100000,
    5,
    5,
    1,
    true,
    '["gemini-2.5-flash-lite", "gemini-3.1-flash-lite", "gemini-2.5-flash", "minimax/minimax-m3:free", "glm-5.3-flash", "thinkingmachines/inkling:free", "nvidia/nemotron-3-ultra-550b-a55b:free"]'::jsonb,
    '["automated_reviews", "custom_rules", "byok_encryption"]'::jsonb
),
(
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
    '["gpt-5.6-luna", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "qwen3.8-max", "kimi-k3", "deepseek-chat", "mistral-large-3", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite", "glm-5.3-flash", "minimax/minimax-m3:free"]'::jsonb,
    '["automated_reviews", "custom_rules", "unlimited_repos", "byok_encryption", "dora_metrics"]'::jsonb
),
(
    'TEAM',
    'Team Plan',
    149900,
    1900,
    14000000,
    700000,
    25,
    0,
    15,
    true,
    '["claude-sonnet-5", "gpt-5.6-terra", "gpt-5.6-luna", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro", "qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite"]'::jsonb,
    '["saml_sso", "scim_provisioning", "custom_rules", "priority_ai_router", "audit_log_cef", "unlimited_repos", "dora_metrics", "byok_encryption"]'::jsonb
),
(
    'SCALE',
    'Scale Plan',
    2499000,
    29900,
    40000000,
    1200000,
    100,
    0,
    30,
    true,
    '["claude-opus-5", "claude-sonnet-5", "gpt-5.6-terra", "gpt-5.6-luna", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro", "qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite"]'::jsonb,
    '["saml_sso", "scim_provisioning", "custom_rules", "priority_ai_router", "audit_log_cef", "unlimited_repos", "dora_metrics", "byok_encryption", "dedicated_workers", "priority_sla"]'::jsonb
),
(
    'ENTERPRISE',
    'Enterprise Custom Plan',
    999900,
    12900,
    100000000,
    2000000,
    0,
    0,
    50,
    true,
    '["claude-opus-5", "claude-fable-5", "claude-sonnet-5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.1-pro", "qwen3.8-max", "kimi-k3", "deepseek-chat", "grok-3", "mistral-large-3", "gemini-2.5-flash-lite", "gemini-3.1-flash-lite"]'::jsonb,
    '["saml_sso", "scim_provisioning", "custom_rules", "priority_ai_router", "audit_log_cef", "unlimited_repos", "dora_metrics", "byok_encryption", "air_gapped", "dedicated_sla"]'::jsonb
)
ON CONFLICT (tier) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    amount_inr = EXCLUDED.amount_inr,
    amount_usd = EXCLUDED.amount_usd,
    monthly_tokens = EXCLUDED.monthly_tokens,
    burst_limit_per_min = EXCLUDED.burst_limit_per_min,
    max_seats = EXCLUDED.max_seats,
    max_repositories = EXCLUDED.max_repositories,
    max_concurrent_reviews = EXCLUDED.max_concurrent_reviews,
    byok_allowed = EXCLUDED.byok_allowed,
    allocated_models = EXCLUDED.allocated_models,
    features_enabled = EXCLUDED.features_enabled,
    updated_at = now();
