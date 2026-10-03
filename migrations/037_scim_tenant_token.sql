-- Migration 037: per-workspace SCIM provisioning token
--
-- Why this exists
-- ---------------
-- ResolveSCIMWorkspace resolved the tenant with:
--
--     SELECT id FROM workspaces ORDER BY created_at ASC LIMIT 1;
--
-- That is a single-tenant assumption. On a multi-tenant deployment every SCIM
-- connection bound to whichever workspace happened to be created first, so one
-- customer's directory sync would provision users into another customer's
-- workspace and consume that workspace's seat quota. The bearer token guarding
-- the endpoint was also the application-wide JWT secret, so it identified no
-- tenant at all.
--
-- Each workspace now holds the hash of its own SCIM token. The presented token
-- is hashed and looked up, and the workspace that owns it scopes the request.
--
-- Only the hash is stored. The plaintext is shown once at issue time and is not
-- recoverable, which is the same posture as an API key.

ALTER TABLE workspaces
    ADD COLUMN IF NOT EXISTS scim_token_hash   TEXT,
    ADD COLUMN IF NOT EXISTS scim_token_prefix VARCHAR(24),
    ADD COLUMN IF NOT EXISTS scim_token_issued_at TIMESTAMPTZ;

-- Lookup happens on every SCIM request, so the hash is indexed. The partial
-- unique index also guarantees two workspaces can never share a token.
CREATE UNIQUE INDEX IF NOT EXISTS workspaces_scim_token_hash_uniq
    ON workspaces (scim_token_hash)
    WHERE scim_token_hash IS NOT NULL;

COMMENT ON COLUMN workspaces.scim_token_hash IS
    'SHA-256 hex of the workspace SCIM provisioning token. Resolves the tenant for a SCIM request; NULL means SCIM is not enabled for this workspace.';
COMMENT ON COLUMN workspaces.scim_token_prefix IS
    'Non-secret leading characters of the SCIM token, shown in the UI so a token can be identified without revealing it.';
COMMENT ON COLUMN workspaces.scim_token_issued_at IS
    'When the current SCIM token was issued. Rotating the token overwrites the hash.';
