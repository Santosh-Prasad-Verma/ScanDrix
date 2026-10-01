# Threat Model (in-repo, code-adjacent)

Boundaries under analysis: public internet → WAF → webhooks/API → RabbitMQ/Redis/PG → workers/sandbox → SCM/IdP/billing callbacks. Out of scope: customer-internal networks beyond our endpoints, provider-side breaches (handled via verification + rotation, not prevention).

## Per-boundary threats (threat → mitigation → residual)

| # | Threat | Mitigation (code pointer) | Residual risk + control |
|---|---|---|---|
| T1 | Forged webhooks (fake PR events, spam reviews) | Per-provider HMAC verification; unverified → 401 drop, never processed (`webhooks` gateway + `*_WEBHOOK_SECRET`) | Secret leak → rotation runbook; document never disables verification under load (`runbooks.md` §2) |
| T2 | Tenant escape (workspace A reads B) | RLS via `app.current_tenant_id` on 43 tables; every query in `internal/database` declares its context (`ExecWithTenant` / `ExecAsSystem`); workspace checks in controllers; live isolation tests (`findings_rls_test.go`, `workspace_scope_test.go`) | **OPEN — see R3 below.** 19 live repositories in a parallel layer (`core/repositories`, `organization/infrastructure`, `platformdata`, `notifications`) still reach RLS tables through a bare pool: 150 calls. Under `scandrix_runtime` these read zero rows / fail writes, so they are functionally broken *and* an unenforced boundary. Inventory and ratchet: `rls_bypass_test.go` |
| T3 | License forgery / tamper | Ed25519 verify, fail-closed boot, hash of envelope; tamper alert logged | Stolen authority private key → revocation + rotation ceremony (TRD §3.3); HSM storage is the control |
| T4 | SCIM bearer theft → mass provision/deprovision | Constant-time compare, dedicated per-workspace token hash lookup (migration 037), zero-downtime rotation ([IMPLEMENTED]), rate limits (SPECCED) | Theft window bounded by rotation interval + anomaly alert on provision bursts |
| T5 | Session/token theft (JWT, CLI keys, refresh) | Short-lived access + refresh rotation, revocation on deprovision, `scandrix_`-prefixed keys with prefix-lookup + hash compare, device quota | XSS/CSRF controls per AGENTS.md baseline; `httpOnly`/`Secure`/`SameSite` cookie flags (dashboard must fix `httpOnly:false` before EE) |
| T6 | Prompt injection via diffs (malicious PR text steering the reviewer) | Treat diff content as untrusted input: instruction hierarchy (system > config > diff), output schema validation, no tool execution from model-proposed commands without sandbox | Residual accepted and disclosed: reviewer output is advisory; merge authority stays human + branch protection |
| T7 | Sandbox escape (malicious patch breaks containment) | Rootless uid 10001, `--net=none`, RO rootfs + tmpfs, cgroup caps, 15s timeout (TRD §3.2) | Kernel-zero-day class residual → run sandboxes on isolated nodes per tier; escape = SEV-1 |
| T8 | Secret exfiltration via logs/errors | No-secret logging rule (AGENTS.md §1.7/§5.7), error sanitization at API boundary | Secret-scanning in CI (gitleaks-class tool) as backstop |
| T9 | Billing webhook replay (double-charge/double-provision) | Signature verify + idempotency store (`billing/idempotency_store.go`) | Clock-skew on timestamps bounded by provider tolerance; replays outside window rejected |
| T10 | Dependency/supply-chain compromise | Lockfiles, `go-licenses` gate, `gosec`, Dependabot-class automation (REQ-8.6) | Pin + audit; release artifacts signed (SPECCED) |

## Database privilege model

Every long-running service connects as `scandrix_runtime` (`NOSUPERUSER`, `NOBYPASSRLS`).
Only the one-shot `scandrix-migrate` job uses the owner role, because `CREATE EXTENSION`
requires superuser. See `docs/LEAST_PRIVILEGE_ROLLOUT.md` for the full rollout record.

### Two contexts, declared per query

A query against an RLS-protected table must run in one of two contexts. Under a
non-superuser role, a query in neither **silently matches zero rows** — it does not error.
That silent zero is the failure this model exists to eliminate.

| Context | Helper | Grants | Use for |
|---|---|---|---|
| Tenant-scoped | `Client.ExecWithTenant(ctx, wsID, fn)` | `app.current_tenant_id` | Anything where the caller has a workspace |
| System worker | `Client.ExecAsSystem(ctx, fn)` | `app.is_system_worker` | Identity resolution before a tenant exists; cron; outbox relay |

Two rules that are easy to get wrong and have both been got wrong in this codebase:

1. **A `pgx.Row` / `pgx.Rows` cannot outlive its transaction.** Returned from the closure
   and scanned afterwards, it fails with `conn busy`. Consume rows inside the closure.
2. **A verification query obeys the same policy as the write.** Counting rows with a bare
   pool call to confirm a write succeeded reports zero. This produced two false test
   failures before it was understood.

### Residual risk

- **`app.is_system_worker` is a broad bypass.** Any code path that sets it can read any row
  in all 43 tables. The guarantee is *"a request-scoped path cannot read another tenant"*,
  not *"no code path can"*. The correct fix is narrow `SECURITY DEFINER` functions plus
  revoking table-level grants from `scandrix_runtime`; that is not done.
- **28 RLS tables have no system-worker branch.** Deliberate: most are only touched inside a
  request that already has a tenant, and adding the branch would grant privilege nothing
  needs. The two that did need it (`warehouse_domain_events`, `sandbox_leases`) were found by
  running under the restricted role and reading errors, not by inspection.
- **The parallel repository layer is the live gap.** See T2 above and
  `LEAST_PRIVILEGE_ROLLOUT.md` §R3.

## Abuse cases (non-technical attackers)

- Seat sharing/multiplexing → license terms + anomaly detection on concurrent sessions (SPECCED analytics).
- Eval gaming (optimizing to the golden corpus) → corpus refresh policy + blind spot-audits (`evaluation/methodology.md` §3).
- Social-engineering of helpdesk SSO → helpdesk tokens are ≤60min, read-only scope; verify out-of-band for seat/billing changes.

## Review cadence

Threat model reviewed per release and after any SEV-1/SEV-2. New endpoint, queue, or secret added without a row here fails review.
