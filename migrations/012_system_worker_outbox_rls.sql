-- Migration 012: System Worker RLS Policy for Outbox Events
-- Allows background system worker processes to query and dispatch outbox events across tenants
-- when app.is_system_worker = 'true' is set in the session/transaction.

DROP POLICY IF EXISTS tenant_isolation_outbox ON outbox_events;
CREATE POLICY tenant_isolation_outbox ON outbox_events
    FOR ALL
    USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
