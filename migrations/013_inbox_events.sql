-- Migration 013: Inbox Deduplicator for Distributed Worker Idempotency
-- Enforces cluster-wide exactly-once message consumption across worker replicas

CREATE TABLE IF NOT EXISTS inbox_records (
    id VARCHAR(255) PRIMARY KEY, -- message_id:consumer_id
    message_id VARCHAR(255) NOT NULL,
    consumer_id VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PROCESSING',
    attempt_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inbox_message_id ON inbox_records(message_id);
