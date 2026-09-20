-- ═══════════════════════════════════════════════════════════════
-- ScanDrix AI - Enterprise Code Review Platform
-- Copyright (c) 2026 ScanDrix AI. All rights reserved.
-- ═══════════════════════════════════════════════════════════════
-- Migration 022: MCP Manager Relational Schema & Tables
-- Creates isolated schema "mcp-manager" and tables for connections,
-- integrations, and OAuth token credentials.
-- ═══════════════════════════════════════════════════════════════

CREATE SCHEMA IF NOT EXISTS "mcp-manager";

-- 1. MCP Connections table
CREATE TABLE IF NOT EXISTS "mcp-manager"."mcp_connections" (
    "id" UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "organizationId" VARCHAR(255) NOT NULL,
    "integrationId" VARCHAR(255) NOT NULL,
    "provider" VARCHAR(100) NOT NULL,
    "status" VARCHAR(50) NOT NULL,
    "appName" VARCHAR(255) NOT NULL,
    "mcpUrl" VARCHAR(1024),
    "allowedTools" JSONB NOT NULL DEFAULT '[]'::jsonb,
    "metadata" JSONB,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "deletedAt" TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS "idx_mcp_conn_org" ON "mcp-manager"."mcp_connections" ("organizationId");
CREATE INDEX IF NOT EXISTS "idx_mcp_conn_org_integration" ON "mcp-manager"."mcp_connections" ("organizationId", "integrationId");
CREATE INDEX IF NOT EXISTS "idx_mcp_conn_status" ON "mcp-manager"."mcp_connections" ("status");

-- 2. Custom MCP Integrations table
CREATE TABLE IF NOT EXISTS "mcp-manager"."mcp_integrations" (
    "id" UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "active" BOOLEAN NOT NULL DEFAULT true,
    "organizationId" TEXT NOT NULL,
    "protocol" VARCHAR(50) NOT NULL DEFAULT 'http',
    "baseUrl" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "logoUrl" TEXT,
    "authType" VARCHAR(50) NOT NULL DEFAULT 'none',
    "auth" TEXT,
    "headers" TEXT,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "deletedAt" TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS "idx_mcp_int_org" ON "mcp-manager"."mcp_integrations" ("organizationId");
CREATE INDEX IF NOT EXISTS "idx_mcp_int_org_active" ON "mcp-manager"."mcp_integrations" ("organizationId", "active");

-- 3. MCP OAuth & Managed Token Credentials table
CREATE TABLE IF NOT EXISTS "mcp-manager"."mcp_integration_oauth" (
    "id" UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    "status" VARCHAR(50) NOT NULL DEFAULT 'INACTIVE',
    "organizationId" TEXT NOT NULL,
    "integrationId" TEXT NOT NULL,
    "auth" TEXT,
    "createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    "updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT "uq_mcp_oauth_org_integration" UNIQUE ("organizationId", "integrationId")
);

CREATE INDEX IF NOT EXISTS "idx_mcp_oauth_org" ON "mcp-manager"."mcp_integration_oauth" ("organizationId");
