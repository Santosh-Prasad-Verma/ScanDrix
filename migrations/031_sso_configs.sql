-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- Migration 031: Per-workspace SSO configuration (SAML / OIDC)
--
-- Replaces the stubbed /sso-config endpoint, which returned a hardcoded
-- response and discarded writes. One row per workspace holds the protocol,
-- the IdP provider configuration, the verified email domains, and the
-- enforcement flags.
--
-- Note on secrets: provider_config may contain an IdP certificate (public)
-- and, for OIDC, a client secret. Client secrets are NOT stored here in
-- plaintext; the repository encrypts provider_config at rest with the
-- existing INTEGRATION_ENCRYPTION_KEY envelope before writing and decrypts
-- on read. See internal/database/sso_config_repository.go.

CREATE TABLE IF NOT EXISTS sso_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

    -- SAML or OIDC. Constrained so a typo cannot persist an unusable config.
    protocol VARCHAR(16) NOT NULL DEFAULT 'SAML'
        CHECK (protocol IN ('SAML', 'OIDC')),

    -- Protocol-specific IdP settings, encrypted at rest by the repository.
    provider_config JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Whether login via this IdP is currently offered.
    active BOOLEAN NOT NULL DEFAULT false,

    -- Verified email domains routed to this workspace (lowercased).
    domains TEXT[] NOT NULL DEFAULT '{}',

    -- When true, password login is refused for this workspace: SSO is the only
    -- accepted authentication method. Enforced on the SSO check/login path.
    saml_required BOOLEAN NOT NULL DEFAULT false,

    -- Set once the IdP handshake has been proven, so an unverified config can
    -- never be the one enforcing logins.
    verified_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly one configuration per workspace. The upsert relies on this.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sso_configs_workspace
    ON sso_configs (workspace_id);

-- Domain routing lookup: "which workspace owns acme.com?"
CREATE INDEX IF NOT EXISTS idx_sso_configs_domains
    ON sso_configs USING GIN (domains);

-- Partial index for the enforcement check on the login path.
CREATE INDEX IF NOT EXISTS idx_sso_configs_enforced
    ON sso_configs (workspace_id)
    WHERE saml_required = true;

-- Row-Level Security (RLS) Tenant Isolation
--
-- ENABLE alone is not enough: the application connects as the table owner, and
-- a table owner bypasses its own RLS policies unless FORCE is set.
ALTER TABLE sso_configs ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_configs FORCE ROW LEVEL SECURITY;

-- Read path: the owning tenant, or the system worker. The system worker needs
-- cross-tenant read for one specific pre-login query - resolving an email domain
-- to its workspace before any session exists - which is why the bypass is
-- spelled the same way as the other tenant tables.
DROP POLICY IF EXISTS tenant_isolation_sso_configs ON sso_configs;
CREATE POLICY tenant_isolation_sso_configs ON sso_configs
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::UUID
    )
    WITH CHECK (
        -- Writes are never widened: even the system worker must name a tenant,
        -- so a background job cannot create an orphan configuration.
        workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::UUID
    );
