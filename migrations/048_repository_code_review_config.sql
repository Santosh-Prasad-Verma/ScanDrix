-- Migration 048: Per-repository code review configuration.
--
-- The dashboard's per-repository settings tabs (custom messages, PR summary,
-- review categories, suggestion control, linked repositories, prompt overrides,
-- ignored title keywords, ...) were being edited in the UI and then dropped
-- before the save request, after which the UI reported success.
--
-- The obvious place to persist them, /parameters/create-or-update-code-review,
-- is NOT usable here: it writes to workspace_parameters keyed by workspace_id
-- alone and ignores the repositoryId in its own request body. Pointing per-repo
-- settings at it would mean saving repository A's configuration overwrites every
-- other repository's.
--
-- tracked_repositories is already the per-repository row and already carries a
-- review_settings JSONB column, so the configuration lives beside it. This keeps
-- the existing tenant RLS and the repository_id scoping intact.
--
-- The seven fixed columns on RepositoryReviewSettings are left in place: they
-- back the CLI and the per-repository policy fields. code_review_config holds the
-- rest of the dashboard's per-repository configuration as one document, so adding
-- a setting does not require a migration.

ALTER TABLE tracked_repositories
    ADD COLUMN IF NOT EXISTS code_review_config JSONB NOT NULL DEFAULT '{}'::jsonb;

-- The configuration is a document, so it must be an object rather than a scalar or
-- an array. An empty object means "nothing configured yet", which is distinct from
-- a NULL column.
ALTER TABLE tracked_repositories
    DROP CONSTRAINT IF EXISTS tracked_repositories_code_review_config_object;

ALTER TABLE tracked_repositories
    ADD CONSTRAINT tracked_repositories_code_review_config_object
    CHECK (jsonb_typeof(code_review_config) = 'object');

COMMENT ON COLUMN tracked_repositories.code_review_config IS
    'Per-repository dashboard code review configuration. Read and written through '
    'GET/PATCH /api/v1/cli/config/repositories/{id}/settings. Never sourced from '
    'workspace_parameters, which is workspace-wide.';