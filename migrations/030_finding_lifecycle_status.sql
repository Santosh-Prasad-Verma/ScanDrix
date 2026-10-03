-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- Migration 030: Finding Lifecycle Status
--
-- The dashboard and repository queries were written against a code_findings
-- table that carries a lifecycle status column, but the column was never
-- created. Every query filtering on cf.status failed at runtime with
-- SQLSTATE 42703, and DismissFinding had been smuggling the state into the
-- remediation column as a "DISMISSED: " string prefix, which destroyed the
-- remediation text it was supposed to preserve.
--
-- This migration adds the column the code already expects.
-- ═══════════════════════════════════════════════════════════════

-- 1. Lifecycle status for a finding. OPEN is the state of every existing row.
ALTER TABLE code_findings
    ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'OPEN';

-- 2. Why a finding was dismissed. The facet query filters on this to separate
--    "still open" from "dismissed by a developer", and the reason was previously
--    concatenated into remediation rather than stored here.
ALTER TABLE code_findings
    ADD COLUMN IF NOT EXISTS dismissal_reason TEXT;

-- 3. Constrain the status to the states the application writes.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'code_findings_status_check') THEN
        ALTER TABLE code_findings
            ADD CONSTRAINT code_findings_status_check
            CHECK (status IN ('OPEN', 'RESOLVED', 'DISMISSED'));
    END IF;
END
$$;

-- 4. Index the filter the dashboard digest and facet queries use.
CREATE INDEX IF NOT EXISTS idx_code_findings_status
    ON code_findings (workspace_id, status);

-- 5. Backfill any finding that was previously dismissed through the
--    remediation-column sentinel, so historical dismissals are not lost. The
--    remediation text those rows overwrote is recovered where the prefix left
--    anything behind.
UPDATE code_findings
SET dismissal_reason = trim(substring(remediation FROM '^DISMISSED:\s*(.*)$'))
WHERE remediation LIKE 'DISMISSED:%'
  AND status = 'OPEN';

UPDATE code_findings
SET status = 'DISMISSED'
WHERE dismissal_reason IS NOT NULL
  AND status = 'OPEN';
