-- =============================================================================
-- Migration 034: outbox visibility timeout
--
-- Problem
-- -------
-- FetchPendingOutboxEvents reclaimed stuck rows with:
--
--     WHERE status = 'PENDING'
--        OR (status = 'PROCESSING' AND created_at < NOW() - INTERVAL '5 minutes')
--
-- That keys off `created_at`, which is when the event was *written*, not when it
-- was *claimed*. A row claimed 4 minutes after it was created and published at
-- 4m30s would be re-claimed the moment the relay restarted, because its
-- `created_at` was already older than the 5 minute window. Events were
-- therefore re-published roughly every 5 minutes regardless of whether the
-- original publish succeeded, and a legitimately slow publish was duplicated.
--
-- Fix
-- ---
-- Track the claim time explicitly. A row is reclaimable only when the claim has
-- actually been outstanding for longer than the timeout, which makes the
-- behaviour independent of how long the event sat in PENDING.
--
-- The timeout is deliberately short (60s). Publishing to RabbitMQ is fast; a
-- relay that cannot publish within a minute is stuck, and re-queuing sooner
-- means a crashed worker's events are recovered in about a minute instead of
-- five. The consumer inbox claim is what prevents duplicate processing.
-- =============================================================================

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS visibility_timeout timestamptz;

-- Rows already stuck in PROCESSING predate the column. Treat them as claimed
-- now so they are retried promptly rather than being judged by created_at.
UPDATE outbox_events
   SET visibility_timeout = NOW()
 WHERE status = 'PROCESSING'
   AND visibility_timeout IS NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_reclaimable
    ON outbox_events (status, visibility_timeout)
    WHERE status = 'PENDING';
