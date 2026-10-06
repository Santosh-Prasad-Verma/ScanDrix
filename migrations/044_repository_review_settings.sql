-- Preserve the existing tenant RLS policy while adding per-repository review policy.
ALTER TABLE tracked_repositories
    ADD COLUMN IF NOT EXISTS review_settings JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Preserve review history when a repository is removed from active tracking.
ALTER TABLE tracked_repositories
    ADD COLUMN IF NOT EXISTS is_tracked BOOLEAN NOT NULL DEFAULT TRUE;
