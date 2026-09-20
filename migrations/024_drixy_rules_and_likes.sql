-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- Migration 024: Drixy Rules Aggregate Storage & Rule Likes Feedback
-- Full schema for drixy_rules and drixy_rule_likes tables.
-- High-throughput GIN indexing for JSONB rule querying,
-- atomic updates, deterministic pattern matching, and voter sentiment.
-- ═══════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS drixy_rules (
    id VARCHAR(255) PRIMARY KEY,
    organization_id VARCHAR(255) NOT NULL,
    rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index on organization_id for fast tenant retrieval
CREATE INDEX IF NOT EXISTS idx_drixy_rules_org_id 
    ON drixy_rules (organization_id);

-- GIN index on rules JSONB for deep structural queries (UUID, path, severity, status, detectors)
CREATE INDEX IF NOT EXISTS idx_drixy_rules_jsonb_gin 
    ON drixy_rules USING GIN (rules);

CREATE TABLE IF NOT EXISTS drixy_rule_likes (
    id VARCHAR(255) PRIMARY KEY,
    rule_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    feedback VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_drixy_rule_user UNIQUE (rule_id, user_id)
);

-- Index on rule_id and feedback for lightning aggregation
CREATE INDEX IF NOT EXISTS idx_drixy_rule_likes_rule_feedback 
    ON drixy_rule_likes (rule_id, feedback);

-- Index on user_id for tracking user interactions
CREATE INDEX IF NOT EXISTS idx_drixy_rule_likes_user 
    ON drixy_rule_likes (user_id);
