package migrations

import (
	"context"
)

// Migration016UsersAndAuthTables represents database migration 016_users_and_auth_tables.
type Migration016UsersAndAuthTables struct{}

// Version returns the unique migration version string.
func (m *Migration016UsersAndAuthTables) Version() string {
	return "016"
}

// Name returns the descriptive name of the migration.
func (m *Migration016UsersAndAuthTables) Name() string {
	return "016_users_and_auth_tables"
}

// Up applies the schema changes defined in 016_users_and_auth_tables.sql.
func (m *Migration016UsersAndAuthTables) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 016: Users and Refresh Token Auth Tables
-- Provisions enterprise authentication tables and enums for ScanDrix runtime

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'users_role_enum') THEN
        CREATE TYPE "public"."users_role_enum" AS ENUM(
            'owner',
            'billing_manager',
            'repo_admin',
            'contributor',
            'admin',
            'member'
        );
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'users_status_enum') THEN
        CREATE TYPE "public"."users_status_enum" AS ENUM(
            'active',
            'inactive',
            'pending',
            'awaiting_approval',
            'removed',
            'pending_email'
        );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS "users" (
    "uuid" uuid NOT NULL DEFAULT gen_random_uuid(),
    "createdAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    "email" character varying NOT NULL,
    "password" character varying NOT NULL,
    "role" "public"."users_role_enum" NOT NULL DEFAULT 'owner',
    "status" "public"."users_status_enum" NOT NULL DEFAULT 'active',
    "organization_id" uuid REFERENCES workspaces(id) ON DELETE CASCADE,
    CONSTRAINT "UQ_users_email" UNIQUE ("email"),
    CONSTRAINT "PK_users_uuid" PRIMARY KEY ("uuid")
);

CREATE TABLE IF NOT EXISTS "auth" (
    "uuid" uuid NOT NULL DEFAULT gen_random_uuid(),
    "userUuid" uuid NOT NULL REFERENCES "users"("uuid") ON DELETE CASCADE,
    "refreshToken" text NOT NULL,
    "expiryDate" TIMESTAMP WITH TIME ZONE NOT NULL,
    "used" boolean NOT NULL DEFAULT false,
    "authProvider" character varying NOT NULL DEFAULT 'credentials',
    "createdAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    "updatedAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT "PK_auth_uuid" PRIMARY KEY ("uuid")
);

CREATE INDEX IF NOT EXISTS "idx_users_email" ON "users"(LOWER("email"));
CREATE INDEX IF NOT EXISTS "idx_auth_refresh_token" ON "auth"("refreshToken");

-- Grant permissions to scandrix_app runtime role
GRANT SELECT, INSERT, UPDATE, DELETE ON "users" TO scandrix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON "auth" TO scandrix_app;`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 016_users_and_auth_tables.sql.
func (m *Migration016UsersAndAuthTables) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS auth, users CASCADE;
DROP TYPE IF EXISTS public.users_role_enum, public.users_status_enum CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
