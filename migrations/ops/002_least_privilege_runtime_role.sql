-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Least-privilege runtime database role
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
--
-- WHY THIS EXISTS
-- ---------------
-- The application historically connected as a SUPERUSER with BYPASSRLS. That
-- makes every Row-Level Security policy in the schema decorative for the
-- application's own connection: PostgreSQL exempts superusers and any role with
-- BYPASSRLS from RLS entirely. Tenant isolation then depends solely on each
-- query remembering to filter on workspace_id.
--
-- This creates a separate runtime role that is subject to RLS, so the policies
-- are actually enforced at the database kernel.
--
-- RUN AS: the schema owner / a superuser, ONCE, before starting services.
-- This is an operations step, not a migration: creating a role needs CREATEROLE,
-- which the runtime role deliberately does not have.
--
-- TWO ROLES
-- ---------
--   scandrix_owner  - owns the schema, runs migrations (needs CREATE EXTENSION
--                     for uuid-ossp / pgcrypto / vector, which is superuser-only).
--   scandrix_runtime- what api / worker / webhooks / server connect as. No
--                     superuser, no BYPASSRLS, no DDL outside the schema CREATE
--                     it needs for its own lazy tables.
--
-- Services read DATABASE_URL; `scandrix-migrate` prefers MIGRATION_DATABASE_URL
-- so the privileged DSN never has to be present in the service containers.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'scandrix_runtime') THEN
        CREATE ROLE scandrix_runtime
            NOSUPERUSER
            NOBYPASSRLS
            NOCREATEDB
            NOCREATEROLE
            NOREPLICATION
            LOGIN;
    END IF;
END
$$;

GRANT CONNECT ON DATABASE scandrix TO scandrix_runtime;

-- USAGE to read types (incl. the pgvector `vector` type); CREATE because two
-- packages lazily ensure their own tables at boot
-- (internal/database/cli_session_store.go, internal/identity/infrastructure/schema.go).
GRANT USAGE, CREATE ON SCHEMA public TO scandrix_runtime;
GRANT USAGE ON SCHEMA "mcp-manager" TO scandrix_runtime;

-- Data access on everything that exists today. Re-run after adding migrations
-- that create new tables, or grant on ALL TABLES IN SCHEMA below instead.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO scandrix_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA "mcp-manager" TO scandrix_runtime;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO scandrix_runtime;

-- Default privileges, so tables created by later migrations are granted
-- automatically and a new table is never silently unreachable.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO scandrix_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO scandrix_runtime;

-- RLS is enforced for this role, so the policies are no longer decorative.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT EXECUTE ON FUNCTIONS TO scandrix_runtime;
