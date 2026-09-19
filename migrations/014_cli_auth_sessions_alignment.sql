-- Migration 014: Align cli_auth_sessions columns with repository and RFC 8628 fields

ALTER TABLE cli_auth_sessions 
    ADD COLUMN IF NOT EXISTS uuid UUID DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS state VARCHAR(255),
    ADD COLUMN IF NOT EXISTS device_code VARCHAR(255),
    ADD COLUMN IF NOT EXISTS user_code VARCHAR(64),
    ADD COLUMN IF NOT EXISTS redirect_uri TEXT,
    ADD COLUMN IF NOT EXISTS mode VARCHAR(32) DEFAULT 'device',
    ADD COLUMN IF NOT EXISTS access_token TEXT,
    ADD COLUMN IF NOT EXISTS refresh_token TEXT,
    ADD COLUMN IF NOT EXISTS user_email VARCHAR(255),
    ADD COLUMN IF NOT EXISTS user_agent TEXT,
    ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS "createdAt" TIMESTAMPTZ DEFAULT now(),
    ADD COLUMN IF NOT EXISTS "updatedAt" TIMESTAMPTZ DEFAULT now();

UPDATE cli_auth_sessions SET uuid = id WHERE uuid IS NULL;
UPDATE cli_auth_sessions SET "updatedAt" = created_at WHERE "updatedAt" IS NULL;
UPDATE cli_auth_sessions SET "createdAt" = created_at WHERE "createdAt" IS NULL;

CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_device_code ON cli_auth_sessions(device_code);
CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_user_code ON cli_auth_sessions(user_code);
CREATE INDEX IF NOT EXISTS idx_cli_auth_sessions_uuid ON cli_auth_sessions(uuid);
