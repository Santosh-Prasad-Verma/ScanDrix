-- Migration 039: Store refresh tokens as SHA-256 digests instead of plaintext
--
-- SECURITY (AUDIT_REMEDIATION.md F-12)
--
-- The `auth` table stored the refresh token verbatim and every lookup was an
-- equality match on that value. A read-only database compromise — a SQL
-- injection, an unencrypted backup, a replica snapshot, a pg_dump pasted into a
-- support ticket, or an over-broad BI read role — therefore yielded live
-- 30-day bearer credentials rather than digests to crack. No cracking was
-- needed: the stored value *was* the credential.
--
-- The fix mirrors what internal/auth/clitokens already does for CLI keys
-- (sha256 of the presented token, models.TokenHash never serialised).
--
-- Migration shape:
--   1. add `tokenHash` CHAR(64)
--   2. backfill it from the existing plaintext so current sessions keep working
--   3. add the lookup index
--   4. drop the plaintext column and its index, so the value cannot be
--      written or read again by any path
--
-- The backfill happens before the drop, so this is a single online operation
-- with no forced logout. Tokens issued after the migration are stored hashed by
-- internal/database/auth_repository.go.

-- ── 1. add the digest column ──────────────────────────────────────────────
ALTER TABLE "auth"
    ADD COLUMN IF NOT EXISTS "tokenHash" CHAR(64);

-- ── 2. backfill existing rows from the plaintext value ───────────────────
-- encode(sha256(...), 'hex') is the same digest the Go code computes with
-- crypto/sha256 + hex encoding, so the two agree exactly.
--
-- Guarded on the column still existing so this file is re-runnable against a
-- database where a previous run already dropped it: without the guard the
-- backfill aborts the whole transaction and the migration cannot complete.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'auth' AND column_name = 'refreshToken'
    ) THEN
        EXECUTE $b$
            UPDATE "auth"
            SET "tokenHash" = encode(sha256(convert_to("refreshToken", 'UTF8')), 'hex')
            WHERE "refreshToken" IS NOT NULL
              AND "refreshToken" <> ''
              AND ("tokenHash" IS NULL OR "tokenHash" = '')
        $b$;
    END IF;
END $$;

-- Any row the backfill could not hash is a session that can no longer be
-- authenticated: the plaintext is gone and there is no digest to look up. Such
-- a row is removed rather than left with a NULL digest, which would violate the
-- NOT NULL invariant below. Deleting it is not a loss — the token it described
-- was already unusable, and the user re-authenticates through the normal flow.
DELETE FROM "auth"
WHERE "tokenHash" IS NULL OR "tokenHash" = '';

-- ── 3. enforce the invariant and index the lookup column ──────────────────
ALTER TABLE "auth"
    ALTER COLUMN "tokenHash" SET NOT NULL;

CREATE INDEX IF NOT EXISTS "idx_auth_token_hash" ON "auth"("tokenHash");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_auth_token_hash_unique" ON "auth"("tokenHash");

-- ── 4. remove the plaintext column ───────────────────────────────────────
DROP INDEX IF EXISTS "idx_auth_refresh_token";
ALTER TABLE "auth" DROP COLUMN IF EXISTS "refreshToken";

-- RLS: the auth table carries bearer credentials, so it must be tenant-scoped
-- like every other credential-bearing table.
ALTER TABLE "auth" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "auth" FORCE ROW LEVEL SECURITY;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_policies
        WHERE tablename = 'auth' AND policyname = 'auth_tenant_isolation'
    ) THEN
        -- users.organization_id is the tenant column (snake_case, unlike the
        -- camelCase columns on auth itself). NULLIF matches the pattern already
        -- used by 033_users_rls.sql so an unset setting cannot be coerced.
        --
        -- The app.is_system_worker clause is REQUIRED, not a convenience:
        -- refresh-token lookup and rotation run before a tenant is known, via
        -- Client.ExecAsSystem, which sets exactly that flag. Without it the
        -- lookup matches no row and every refresh returns
        -- "invalid or expired refresh token".
        EXECUTE $p$
            CREATE POLICY auth_tenant_isolation ON "auth"
                USING (
                    current_setting('app.is_system_worker', true) = 'true'
                    OR "userUuid" IN (
                        SELECT "uuid" FROM "users"
                        WHERE organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
                    )
                )
                WITH CHECK (
                    current_setting('app.is_system_worker', true) = 'true'
                    OR "userUuid" IN (
                        SELECT "uuid" FROM "users"
                        WHERE organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
                    )
                )
        $p$;
    END IF;
END $$;

COMMENT ON COLUMN "auth"."tokenHash" IS
    'SHA-256 hex digest of the refresh token. The plaintext token is never stored; it is only ever held in memory for the response that issues it.';
