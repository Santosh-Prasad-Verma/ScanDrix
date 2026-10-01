-- =============================================================================
-- Migration 036: allow the system worker on the warehouse event store
--
-- Found by the least-privilege E2E run: with the worker on scandrix_runtime the
-- DORA rollup cron failed on boot with
--
--   ERROR: new row violates row-level security policy for table
--   "warehouse_domain_events" (SQLSTATE 42501)
--
-- Cause
-- ------
-- Unlike the other 42 RLS-protected tables, this policy had no system-worker
-- branch at all:
--
--   USING (workspace_id = current_tenant_id)
--
-- The DORA rollup is a cross-tenant aggregate. It inserts one event per active
-- workspace in a single statement, so it has no single tenant to run under and
-- was permanently unable to write.
--
-- Fix
-- ---
-- Give the policy the same system-worker bypass the rest of the schema uses.
-- The rollup runs through Repository.AggregateDORARollup, which already goes
-- through ExecAsSystem, so this restores the intended behaviour without
-- widening access for any request-scoped path.
--
-- Scope note
-- ----------
-- A wider audit found 28 RLS tables whose policies lack this branch. They were
-- deliberately NOT all changed: most are only ever read inside a request that
-- already has a tenant, so adding a bypass would grant privilege without need.
-- Tables are widened individually, with the background job that requires it
-- named in the reason.
-- =============================================================================

DROP POLICY IF EXISTS tenant_isolation_warehouse_events ON warehouse_domain_events;

CREATE POLICY tenant_isolation_warehouse_events ON warehouse_domain_events
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    )
    WITH CHECK (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
