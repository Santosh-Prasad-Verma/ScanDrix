-- Migration 046: One seat-allocation row per workspace.
--
-- UpgradeWorkspacePlanAtomic upserted organization_billing_seats using
-- ON CONFLICT (id) DO NOTHING while passing a freshly generated uuid for id, so
-- the conflict could never fire. Every checkout retry and every duplicate
-- webhook inserted another row for the same workspace, inflating
-- allocated_seats and the seat entitlement derived from it.
--
-- The table is per-workspace (it holds that workspace's seat allocation), so
-- one row per workspace is the intended cardinality.
--
-- RLS NOTE: organization_billing_seats is ENABLE + FORCE ROW LEVEL SECURITY
-- (migration 008) and its only policy is tenant_isolation_billing_seats, which
-- admits rows for app.current_tenant_id alone. A migration runs with no tenant
-- context, so a plain DELETE here matches nothing, silently leaves the
-- duplicates in place, and the CREATE UNIQUE INDEX then aborts with
-- "duplicate key value violates unique constraint". The policy below mirrors
-- the app.is_system_worker bypass already used for outbox_events (migration
-- 012) and users (migration 033) so this maintenance statement can see every
-- tenant's rows.

DROP POLICY IF EXISTS billing_system_maintenance ON organization_billing_seats;
CREATE POLICY billing_system_maintenance ON organization_billing_seats
    FOR ALL
    USING (current_setting('app.is_system_worker', true) = 'true')
    WITH CHECK (current_setting('app.is_system_worker', true) = 'true');

DO $$
BEGIN
    -- Keep the most recent row per workspace before enforcing the constraint.
    -- Existing duplicates are collapsed rather than deleted blindly so the
    -- unique index applies cleanly on databases that already accumulated extras.
    PERFORM set_config('app.is_system_worker', 'true', true);

    DELETE FROM organization_billing_seats older
    USING organization_billing_seats newer
    WHERE older.workspace_id = newer.workspace_id
      AND (older.updated_at, older.created_at, older.id)
          < (newer.updated_at, newer.created_at, newer.id);
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS organization_billing_seats_workspace_unique
    ON organization_billing_seats(workspace_id);