# Scandrix — Row-Level Security Contract

**Status:** Normative companion to [DATA-MODEL.md](DATA-MODEL.md)  
**Applies to:** API, worker, webhook relay, remediation worker, and every direct PostgreSQL data path

## Security objective

Every query against tenant-owned data is constrained by the authenticated tenant identifier bound to the current database transaction. RLS is a final database boundary; authorization in Go is still required, but an authorization defect must not permit a cross-tenant row read or write.

The canonical DDL enables and forces RLS on `tenants`, `repositories`, `scans`, `scan_stage_results`, `evidence_packets`, `findings`, `security_memory`, `remediation_attempts`, `assurance_manifests`, `agent_action_ledger`, and `integration_outbox`.

## Context binding

The request authenticator derives `tenant_id` from a verified credential. It never accepts a request header, body field, or query parameter as a database tenant context. The repository layer opens a transaction, applies the context with `is_local = true`, and performs all tenant queries before committing.

```sql
BEGIN;
SELECT set_config('app.current_tenant_id', $1::text, true);
SELECT app.require_tenant_context();

-- Tenant-owned queries and mutations execute here.

COMMIT;
```

`set_config(..., true)` is transaction-local, equivalent to `SET LOCAL`. This is mandatory with PgBouncer transaction pooling: connection state from one request cannot leak to the next request. The context is intentionally absent outside an open transaction; `app.current_tenant_id()` then returns `NULL`, so every RLS predicate is false.

## Policy shape

Each tenant-owned table uses a *permissive* policy with both `USING` and `WITH CHECK` predicates:

```sql
CREATE POLICY findings_tenant_isolation ON findings
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());
```

`USING` controls existing rows visible to `SELECT`, `UPDATE`, and `DELETE`. `WITH CHECK` controls proposed rows for `INSERT` and `UPDATE`. Both are necessary: a read-only predicate alone can allow a tenant-scoped row to be reassigned during an update.

Do not convert this policy to restrictive-only. PostgreSQL combines permissive policies with `OR` and restrictive policies with `AND`; a table with no applicable permissive policy has no accessible rows. A restrictive policy may be added as an additional guard only when a base permissive tenant policy remains in place.

## Role boundary

The migration owner and break-glass administrator are not runtime identities. The runtime role must satisfy all of the following:

| Control | Required state |
| --- | --- |
| Ownership | Does not own tenant tables, sequences, or schema objects. |
| Privilege escalation | `NOSUPERUSER`, `NOCREATEDB`, `NOCREATEROLE`, `NOREPLICATION`, and no `BYPASSRLS`. |
| Network exposure | Accessible only from trusted application workloads through the database pooler; never exposed to browser clients. |
| DDL | No `CREATE`, `ALTER`, `DROP`, or function-replacement permissions in `public` or `app`. |
| Partition access | Queries use the `scans` parent relation. Direct partition privileges are not granted. |

`FORCE ROW LEVEL SECURITY` is applied to each table because otherwise a table owner bypasses its own policies. Superusers and roles with `BYPASSRLS` always bypass RLS by PostgreSQL design; those credentials belong only to audited administrative workflows.

## Runtime grants

After provisioning the non-owner application role, the database administrator grants only the required DML privileges. The example role name is `scandrix_app` and must be substituted only if the deployed runtime role uses a different name.

```sql
REVOKE ALL ON SCHEMA public, app FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC;

GRANT USAGE ON SCHEMA public, app TO scandrix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE
    tenants,
    repositories,
    scans,
    scan_stage_results,
    evidence_packets,
    findings,
    security_memory,
    remediation_attempts,
    assurance_manifests,
    agent_action_ledger,
    integration_outbox
TO scandrix_app;

GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO scandrix_app;
GRANT EXECUTE ON FUNCTION app.current_tenant_id() TO scandrix_app;
GRANT EXECUTE ON FUNCTION app.require_tenant_context() TO scandrix_app;
REVOKE ALL ON FUNCTION app.ensure_scans_partition(timestamptz) FROM scandrix_app;
```

The runtime role has no `UPDATE` or `DELETE` capability over `evidence_packets` or `agent_action_ledger` in a hardened deployment; grants for those tables should be split further if only append operations are needed. Their immutable triggers reject mutation even if an operational role is temporarily over-granted.

## Verification cases

Integration tests run as the runtime role and seed data as a separate owner role. They must prove all four cases:

| Case | Setup | Expected result |
| --- | --- | --- |
| Missing context | Open transaction without `set_config` | `SELECT` returns zero tenant rows; `INSERT` is rejected by RLS. |
| Correct context | Bind tenant A | Reads and writes are limited to tenant A. |
| Wrong context | Bind tenant B while targeting A data | `SELECT`, `UPDATE`, and `DELETE` affect zero A rows; cross-tenant `INSERT` is rejected. |
| Pool reuse | Commit an A transaction and start a new transaction on the same connection | The next transaction has no tenant context until explicitly set. |

The following SQL gives an executable missing-context check after the test fixture has created tenant data:

```sql
BEGIN;
RESET app.current_tenant_id;
SELECT count(*) AS visible_tenants FROM tenants;
ROLLBACK;
```

`visible_tenants` must be `0` for `scandrix_app`. A test failure is a release blocker because tenant isolation is not a best-effort application feature.
