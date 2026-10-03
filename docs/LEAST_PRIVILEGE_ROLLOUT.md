# Least-Privilege Rollout — Status and Remaining Work

**Date:** 2026-09-27
**Scope:** Moving every long-running ScanDrix service off the database superuser and onto a
non-superuser role with RLS actually enforced, plus the correctness work that surfaced once
the real permission model was in place.

This document is the handoff. It states what is verified, what is not, what each remaining
item needs, and exactly how to do and verify it. Nothing here is aspirational: every "verified"
claim was observed against the running stack, and every unverified surface is called out as
unverified.

---

## 1. Status at a glance

| | |
|---|---|
| Long-running services on a superuser role | **0** (was 3: api, webhooks, worker) |
| Bare-pool call sites in worker-path repositories | **0** (`review_`, `audit_`, `ast_graph_`) |
| Bare-pool call sites in `auth_repository.go` | **0 of 29** |
| Live repositories still bypassing RLS | **19 files, 150 calls** — see R3 |
| Tables with RLS enabled | 43 |
| RLS tables with no policy | 0 |
| RLS tables lacking a system-worker branch | 28, deliberately (see R3) |
| Test suite | **291 packages, 0 failing** |
| `users` table RLS | enabled, forced, verified live |
| Migrations 033–036 | applied |
| Review run under `scandrix_runtime` | `COMPLETED`, 1 HIGH finding, 0 RLS errors |

### Role split now in force

| Service | Role | Why |
|---|---|---|
| `scandrix-migrate` | `scandrix_app` (owner/superuser) | one-shot; needs `CREATE EXTENSION` |
| `scandrix-api` | `scandrix_runtime` | RLS enforced |
| `scandrix-webhooks` | `scandrix_runtime` | RLS enforced |
| `scandrix-worker` | `scandrix_runtime` | RLS enforced |

Wiring: `docker-compose.yml`. Services receive `DATABASE_RUNTIME_URL`, built from
`SCANDRIX_RUNTIME_USER` / `SCANDRIX_RUNTIME_PASSWORD`. Compose **fails to start** if
`SCANDRIX_RUNTIME_PASSWORD` is unset — deliberate, per Master Rule 1.6.

---

## 2. The two contexts every query must declare

This is the mental model the whole change rests on. A query against any RLS-protected table
must run in one of two contexts, and under a non-superuser role a query in neither context
**silently matches zero rows**. That silent zero is the failure mode this rollout exists to
eliminate.

| Context | Helper | Sets | Use for |
|---|---|---|---|
| Tenant-scoped | `Client.ExecWithTenant(ctx, wsID, fn)` | `app.current_tenant_id` | Anything where the caller has a workspace. RLS admits only that tenant's rows. |
| System worker | `Client.ExecAsSystem(ctx, fn)` | `app.is_system_worker` | Identity resolution that happens *before* a tenant exists: login by email, refresh tokens, CLI keys by hash, device codes, cron jobs, the outbox relay. |

**Two hard rules learned the hard way during this work:**

1. **A `pgx.Row` or `pgx.Rows` cannot outlive its transaction.** Returning one from the
   closure and scanning afterwards fails with `conn busy`. Consume rows *inside* the closure.
2. **A count/verification query is subject to the same policy as the write.** Counting rows
   with a bare pool call to confirm a write succeeded will report zero. This produced two
   false failures during testing, both in test code, both fixed by scoping the assertion.

---

## 3. Verified — done and evidenced

### 3.1 Repository conversions (the worker-flip blocker)

All 15 call sites across `internal/database/{review,audit,ast_graph}_repository.go` converted.
`r.client.Pool` no longer appears as a **call** in those files (only in nil guards).

Notable decisions:

- `GetWorkspaceByID` / `UpdateWorkspace` needed **no signature change** — the workspace id *is*
  the tenant id, so they self-scope via `ExecWithTenant(ctx, id, …)`.
- `teams` and `team_members` gained a `wsID` parameter, and a `JOIN teams ON t.workspace_id = $2`
  predicate was added so a caller cannot read another tenant's roster by guessing a team id.
- `code_ast_nodes` / `code_ast_edges` have no `workspace_id`; RLS resolves ownership through
  `tracked_repositories`. Added `WorkspaceIDForRepository` (system reverse lookup) to supply
  the tenant.
- **The AST batch inserts were the subtle one.** `pgxpool.SendBatch` on the pool bypasses the
  transaction, so RLS rejects every row. They now run `tx.SendBatch` *inside* `ExecWithTenant`.
- Genuinely cross-tenant functions (`ListWorkspaces`, `CreateWorkspace`,
  `GetWorkspaceIDByRepoNamespace`, `GetPendingApprovalReviews`, `AggregateDORARollup`,
  `GetRepositoryReportsData`) became `ExecAsSystem`, each with a comment stating why.

### 3.2 `auth_repository.go` — all 29 call sites

| | Count |
|---|---|
| Tenant-scoped (`ExecWithTenant`) | 7 |
| System-elevated (`ExecAsSystem`) | 13 |
| Via `execTenant` / `execSystem` / `queryTenant` / `querySystem` helpers | 9 |

The 13 system-elevated ones are identity lookups that cannot know a tenant yet. Each still
filters on an explicit identity predicate, so elevation widens the *RLS context*, not the
result set.

### 3.3 `users` RLS (migration 033)

The highest-value table was the one with no RLS. Verified live under `scandrix_runtime`:

| Context | Rows returned |
|---|---|
| No tenant context | 0 |
| Correct tenant | 1 |
| System worker | 14 |

The pre-authentication path still works: register `201`, login `200`, wrong password `401`,
`/auth/me` `200`. The three pre-tenant lookups already ran through `ExecAsSystem`, so enabling
RLS did not break login.

**Residual, stated plainly:** `app.is_system_worker` is a broad bypass. Any code path that sets
it can read any user, exactly as it can for all 43 RLS tables. The guarantee today is "a
request-scoped code path cannot read another tenant's users", not "no code path can". Narrowing
this requires `SECURITY DEFINER` lookup functions plus revoking table-level grants — see R3.

### 3.4 `warehouse_domain_events` (migration 036)

Found by the least-privilege run. The DORA rollup cron failed on boot with
`new row violates row-level security policy`. This table's policy was the only one of 43
without a system-worker branch, and the rollup writes one event per active workspace, so it
could never succeed. Fixed by adding the standard branch.

### 3.5 `sandbox_leases` — 14 methods

Also found by the run. `internal/sandbox/lease/repository.go` wrote through a raw
`*pgxpool.Pool` with no RLS context, so *every* lease operation was broken under least
privilege. Only `UpsertAcquire` surfaced, because it is the only one the review path calls.
The repository now takes `*database.Client` and routes all 14 through
`execSystem` / `querySystem`. Verified: lease errors under `scandrix_runtime` went **1 → 0**.

### 3.6 SCIM Groups (migration 035)

`scim_groups` + `scim_group_members` replaced an in-memory map that lost all group state on
restart and diverged between replicas. Membership is relational so a removal is one `DELETE`.

Verified live: create `201` → **API restart** → group still listed with both members →
PATCH add member (3) → PATCH rename → DELETE `204` → GET `404`.

### 3.7 Outbox reclaim (migration 034)

The old query reclaimed with `created_at < NOW() - INTERVAL '5 minutes'`, which keys off when
the event was *written*, not when it was *claimed*. A row claimed at 4m30s was re-published on
the next relay pass. Now stamped with `visibility_timeout` at claim time, 60-second window.

### 3.8 Health semantics

`internal/core/health` was dead code. It now backs `/healthz`, `/readyz`, `/livez`. Proven by
stopping Postgres: `/healthz` and `/readyz` → `503`, `/livez` → **stays `200`** (so a database
blip does not get the container killed), recovery → `200`. The previous inline handler
defaulted `dbStatus` to `"ok"` when the repo was nil and reported healthy with no database.

### 3.9 Tenant leak: `ListWorkspaces`

`ListWorkspaces` returns every active workspace with **no per-user filter**. Four API
controllers used `wsList[0]` to pick a default workspace or tenant name, so any authenticated
user could be handed an arbitrary tenant. Fixed:

- `ListWorkspacesForUser(ctx, email)` — the join is on email because `account_profiles` has no
  `user_id` column. (The first version I wrote joined on `user_id` and would have failed at
  runtime; it was never exercised because nothing called it.)
- `auth.CallerEmail(ctx)` added so controllers scope by the caller.
- The org repo adapter's `Find()` returns nothing when no caller is in context, rather than
  everything.
- Only the two cron jobs still call `ListWorkspaces`, correctly.

`internal/database/workspace_scope_test.go` proves it live with two tenants: alice sees only
hers, bob only his, and empty / whitespace / unknown identity all return **zero** workspaces.
It also asserts `ListWorkspaces` returns strictly more, so the test cannot pass vacuously.

### 3.10 Bugs found by the E2E run (not by any test)

| # | Bug | Why it was invisible |
|---|---|---|
| 1 | Reviews **never persisted**. `RepositoryID` was never populated, so the FK failed; the error was discarded with `_ =`. | Pipeline logged "completed successfully" while writing zero rows. |
| 2 | `conn busy` at 3 sites — `pgx.Rows` iterated after commit. | Introduced by this work; caught by grepping the error instead of trusting the success log. |
| 3 | `warehouse_domain_events` policy had no system-worker branch. | DORA cron failed on every boot in production. |
| 4 | `sandbox_leases` — 14 methods through a raw pool. | Non-fatal by design; the review continued self-contained. |

Bug 1 is fixed by resolving `RepositoryID` from the repo namespace and by making `CreateReview`
failure abort loudly with the likely cause named.

---

## 4. Remaining work

### R1 — Verify SCM comment posting

**Status:** not verified. Never exercised in any run.

**Why:** the available GitHub fine-grained PAT is scoped `Public repositories (read-only)`. The
worker reaches `PostInlineComments` / `PostReviewSummary` / `SetCommitStatus` at
`internal/review/orchestrator.go:650-658` and all three fail with `403`. That is expected and
did not affect the DB-layer proof, because the writes land first (lines 192, 547, 558).

**Required:** a token with write access on a private repository.

| Permission | Access |
|---|---|
| Contents | Read and write |
| Pull requests | Read and write |
| Issues | Read and write |
| Commit statuses | Read and write |
| Metadata | Read-only (auto-granted) |

Repository access → **Only select repositories** → the throwaway repo. Keep the shortest
expiry that works.

**How to do it**

1. Create a **private** repo under a throwaway GitHub account, not a production repo. A review
   posts real inline comments and real check runs visible to everyone on the repo.
2. Add a README on `main`, branch `feature/test-review`, add a file that trips Go rules from
   `internal/rules/catalog/golang.go` — e.g. a bare `panic()` (rule: *Direct `panic()` Call in
   Production Service Code*) or `context.WithTimeout` without `defer cancel()`. This matters:
   a hardcoded password produces **zero** findings, because Go's 17 rules cover injection,
   error handling and concurrency and include no secret-detection rule.
3. Open the PR. Record `owner/repo` and the PR number.
4. Store the integration and tracked repository through the repository methods, which apply KMS
   envelope encryption (`internal/database/config_repository.go`):
   - `Repository.UpsertIntegrationConnectionWithSecret(ctx, wsID, "github", owner, token, "", true, 1)`
   - `Repository.TrackRepository(ctx, wsID, "github", namespace, namespace, "main")`
5. Enqueue a `ReviewTaskPayload` on `outbox_events` with `status='PENDING'`. The relay picks it
   up within ~2 seconds and publishes to `scandrix.reviews.v1`.
6. Confirm in the GitHub UI: inline comments on the diff, a review summary, and a
   `scandrix/review` commit status.

**How to verify it worked**

- `pull_request_reviews.findings_count > 0`
- `code_findings` has rows for that `review_id`
- Check runs appear on the PR with the expected conclusion
- The worker log shows `Successfully published review comments to SCM` and no
  `Failed posting platform review summary`

**Risk:** real comments on a real repo. Use a throwaway account.

---

### R2 — Verify LLM synthesis — **DONE**

**Status:** verified. Previously blocked on the OpenRouter free-tier cap.

**Result, both roles, same job, same PR:**

| | `scandrix_app` (owner) | `scandrix_runtime` (least privilege) |
|---|---|---|
| `pull_request_reviews.state` | `COMPLETED` | `COMPLETED` |
| `findings_count` | 1 | 1 |
| `code_findings` rows | 1 | 1 |
| Severity | HIGH | HIGH |
| RLS / permission errors | 0 | 0 |
| Wall clock | 21s | 23s |

The finding was `[HIGH] Hardcoded credential in source code` (runtime) /
`[HIGH] Hardcoded password exposed in source code and logs` (owner). The wording differs
because LLM output is non-deterministic; the count, severity and terminal state are identical,
which is what the least-privilege claim rests on.

**What this newly proves.** This is the first run to reach `COMPLETED`, and it exercises the
write path end to end through the real pipeline rather than through a direct test:

- `BatchInsertFindings` under `scandrix_runtime` — previously only proven by
  `TestBatchInsertFindingsUnderRuntimeRole` against a live database.
- AI synthesis, rule evaluation, blast-radius/call-graph scoring, and both provenance
  attestations (in-toto DSSE and SLSA v1.0) all complete under least privilege.
- Cost of the verification: **$0.039** total on the OpenRouter account.

**Still failing, and expected:** `Failed posting platform review summary — 403`. The token is
read-only, so `PostReviewSummary` / `PostInlineComments` / `SetCommitStatus` cannot succeed.
That is R1, and it is independent of this result: the DB layer is proven, the SCM write path is
not.

**Reproducing it**

```bash
export DATABASE_URL="postgres://scandrix_app:$(grep -E '^POSTGRES_PASSWORD=' .env | cut -d= -f2-)@localhost:5433/scandrix?sslmode=disable"
export SCM_TEST_TOKEN="<read-only token>"
go run ./cmd/e2e-setup          # stages workspace + tracked repo + encrypted integration

# then run the worker with these in the environment:
#   OPENAI_API_KEY, OPENAI_BASE_URL, API_LLM_PROVIDER_MODEL
# enqueue a ReviewTaskPayload on outbox_events with status='PENDING'
```

Note: `OPENAI_API_KEY` in this project's `.env` is an **OpenRouter** key (`sk-or-v1…`) and only
works together with `OPENAI_BASE_URL`. The same key against `api.openai.com` returns 401.

**One thing to know for fixture design.** A file containing a hardcoded password *is* caught by
the AI path, even though Go's static rules have no secret-detection rule
(`internal/rules/catalog/golang.go` covers injection, error handling and concurrency). So a
rules-only run reports zero findings on such a file, and that is correct behaviour rather than a
broken fixture. To exercise the static-rule path specifically, use code that trips one of those
17 rules.

### R3 — Parallel repository layer bypasses RLS

**Status:** open. Now **measured and ratcheted**, previously neither.

**What the audit found.** The original framing of this item was "28 RLS tables lack a
system-worker branch". That was the wrong question, and answering it properly changed the size of
the problem. A repository holds its database handle as `pool *pgxpool.Pool`, not
`client.Pool`, so the earlier scan — which grepped for `r.client.Pool` — missed an entire
parallel data-access layer.

Audited result: **19 live repositories, 150 bare-pool call sites**, reaching RLS-protected
tables including `users`, `teams`, `team_members`, `team_cli_key`, `workspaces`, `parameters`,
`workspace_parameters`, `tracked_repositories`, `integration_connections`,
`platform_pull_requests` and `notification_channels`.

Top of the list:

| Repository | Calls |
|---|---|
| `core/repositories/repository_scm.go` | 18 |
| `core/repositories/tenancy_repository.go` | 16 |
| `platformdata/infrastructure/repositories/postgres_repository.go` | 15 |
| `core/repositories/auth_sso_repository.go` | 9 |
| `organization/…/postgres_parameters_repository.go` | 9 |
| `core/repositories/audit_automation_repository.go` | 8 |
| `core/repositories/drixy_rules_repository.go` | 8 |
| `core/repositories/team_access_repository.go` | 8 |
| `core/repositories/billing_license_repository.go` | 7 |
| `organization/…/postgres_team_repository.go` | 7 |
| `organization/…/postgres_team_member_repository.go` | 7 |
| `organization/…/postgres_team_cli_key_repository.go` | 6 |
| `organization/…/postgres_global_parameters_repository.go` | 6 |
| `organization/…/postgres_cli_device_repository.go` | 5 |
| `organization/…/postgres_organization_parameters_repository.go` | 5 |
| `organization/…/postgres_organization_repository.go` | 5 |
| `organization/…/postgres_user_account_repository.go` | 5 |
| `core/repositories/parameters_preset_repository.go` | 5 |
| `organization/…/postgres_tracked_repository_reader.go` | 1 |

Full list with counts lives in `internal/database/rls_bypass_test.go` as `knownBypass`.

**Why it matters, stated precisely.** Under `scandrix_runtime` a query with neither GUC set
**matches zero rows without erroring**. So these repositories are simultaneously:

- **Functionally broken** — any read returns empty, any write is rejected by the `WITH CHECK`.
  Features backed by them are likely already degraded in the current environment, and no log
  line says so.
- **An unenforced boundary** — whichever service owns the connection is effectively a
  reader of nothing, but the code reads as if it has access. The next person to add a
  grant, or a policy, inherits an assumption that is not true.

**What was done now**

1. `internal/database/rls_bypass_test.go` — the ratchet. It fails if the set of bypassing
   repositories **grows**, fails if any bare-pool call appears in `internal/database` (which
   must stay clean), and logs when a known bypass disappears so the inventory can be tightened.
2. `TestRLSTableInventoryIsCurrent` — compares the `RLS_TABLES` list in that file against the
   live database when `SCANDRIX_E2E_RUNTIME_DSN` is set. It immediately caught three missing
   tables (`drixy_embedding_vectors`, `global_parameters`, `user_repository_assignments`) and
   one stale entry (`cli_auth_sessions`), which are now corrected.
3. `docs/security/threat-model.md` T2 and `docs/architecture/data-model.md` §3 now state the
   gap instead of describing a fully-enforced contract.

**How to finish it.** Per repository, converting in this order:

1. Decide the context per method, not per file. Most are request-scoped and need
   `ExecWithTenant` with the workspace from the request; a minority are identity lookups that
   need `ExecAsSystem`. `NewPostgresUserAccountRepository` and `NewPostgresTeamCliKeyRepository`
   are the latter — they resolve identity before a tenant is known.
2. Change the struct to hold `*database.Client` rather than `*pgxpool.Pool`, and update the
   constructor call sites in the composition roots. The `sandbox/lease` conversion in §3.5 is
   the worked example: 14 methods, one constructor change, no call-site churn beyond the two
   `cmd/` files.
3. Consume rows inside the transaction. Do not return `pgx.Row` / `pgx.Rows` from the closure.
4. Re-run `TestNoNewRLSBypass`. It will pass immediately with one fewer entry in `knownBypass`;
   delete that entry so the ratchet tightens behind you.
5. Verify under the runtime role: restart the owning service, exercise the affected endpoints,
   and assert `pg_stat_activity` shows no superuser session and the log shows no RLS error.

**Do not fix this by adding system-worker branches to the affected tables.** That would make
the queries pass while leaving them cross-tenant and unenforced — turning a loud failure into
a silent data leak. The conversion is the fix; the policy change would be the cover-up.

**The stronger end state, still not attempted.** Replace the `app.is_system_worker` GUC with
narrow `SECURITY DEFINER` functions and revoke table-level grants from `scandrix_runtime`, so a
system-elevated path can only invoke the specific operations it needs. That is a larger change
and should be its own piece of work with its own review.

### R4 — Documentation is now stale

**Status:** open.

`docs/security/threat-model.md` describes RLS generically ("RLS `app.current_tenant_id` on all
tables") and predates the role split. Three docs touch this area and none of them document the
current model:

- `docs/security/threat-model.md` — mitigation table predates `users` RLS, the role split, and
  the system-worker model. It also does not mention the `app.is_system_worker` bypass as a
  residual risk.
- `docs/enterprise/TRD.md` — no mention of `scandrix_runtime` or the least-privilege posture.
- `docs/architecture/data-model.md` — no RLS policy inventory.

**What to add**

1. The two-context model (section 2 of this document) as the canonical explanation.
2. The role split table and the `SECANDRIX_RUNTIME_*` environment contract, including that
   compose fails closed without the password.
3. An RLS inventory: 43 tables enabled, 0 without a policy, 28 without a system-worker branch
   (with the rationale that it is per-need, not blanket).
4. Migrations 033–036 in the architecture/upgrade docs.
5. The residual `app.is_system_worker` risk, stated in the threat model rather than omitted.

---

### R5 — Restore the E2E staging script

**Status:** done.

`cmd/e2e-setup/` staged a workspace, tracked repository and encrypted integration connection
for the E2E run. It went through the real repository methods, so it exercised the same
encryption path as the API.

**Done.** Restored at `cmd/e2e-setup/main.go` as a documented local tool. It is referenced by
neither `Dockerfile` nor `docker-compose.yml`, so it stays out of every service image. It reads
the token from the environment, never prints it, and writes it only through
`UpsertIntegrationConnectionWithSecret`, so the KMS envelope path is exercised rather than
bypassed. Verified end to end: it staged a workspace, tracked repository and encrypted
integration, and the workspace cascaded away cleanly on teardown.

---

## 5. Reproducing the verification

Both live tests skip unless `SCANDRIX_E2E_RUNTIME_DSN` is set, and both **assert the role is not
privileged** first, so they cannot pass vacuously against a superuser.

```bash
export SCANDRIX_E2E_RUNTIME_DSN="postgres://scandrix_runtime:$(grep -E '^SCANDRIX_RUNTIME_PASSWORD=' .env | cut -d= -f2-)@localhost:5433/scandrix?sslmode=disable"

# findings write path + role assertion
go test ./internal/database/ -run TestBatchInsertFindingsUnderRuntimeRole -v -count=1

# tenant scoping: empty/unknown identity must return zero workspaces
go test ./internal/database/ -run TestListWorkspacesIsScopedToCaller -v -count=1

# whole suite
go test ./...
```

Runtime check that no service is privileged:

```bash
docker exec scandrix-postgres psql -U scandrix_app -d scandrix -tAc \
  "select usename, count(*) from pg_stat_activity
   where datname='scandrix' and pid<>pg_backend_pid() group by usename;"
# expect only scandrix_runtime
```

Fail-closed health check:

```bash
docker stop scandrix-postgres
curl -o /dev/null -w '%{http_code}\n' localhost:8080/readyz   # 503
curl -o /dev/null -w '%{http_code}\n' localhost:8080/livez    # 200
docker start scandrix-postgres
```

**Note on the test harness:** `internal/review` and `test/integration` each have a `TestMain`
that clears every LLM provider variable. Without it, the gateway honours
`OPENAI_BASE_URL` from `.env` over the test's own mock server and issues **real paid API
calls**. Do not remove those `TestMain` blocks.

---

## 6. Credentials and configuration checklist

| Variable | Service | Where to get it | Status |
|---|---|---|---|
| `SCANDRIX_RUNTIME_PASSWORD` | compose | generated locally; any strong value | **required** — compose fails closed without it |
| `SCANDRIX_RUNTIME_USER` | compose | `scandrix_runtime` | set |
| `DATABASE_RUNTIME_URL` | compose | escape hatch for managed Postgres | optional, unset locally |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | migrate only | local DB owner | set |
| `MIGRATION_DATABASE_URL` | migrate | privileged DSN | set by compose |
| `SCIM_BEARER_TOKEN` | api, webhooks, worker | any random string | set |
| SCM token (write-scoped) | worker | see R1 | **needed for R1** |
| LLM key + base URL + model | worker | see R2 | **needed for R2** |

Never place any of these in a tracked file. `.env` is gitignored; `.env.example` carries names
and placeholders only.

---

## 7. Rollback

If least privilege causes an incident:

1. Set `DATABASE_RUNTIME_URL` to the owner DSN in `.env`, or comment the `DATABASE_RUNTIME_URL`
   override out of `docker-compose.yml` for the affected service. Compose falls back to the
   `POSTGRES_*` DSN.
2. `docker compose up -d <service>`.

The RLS policies stay in place, so nothing has to be reverted in the database. No data
migration is involved in either direction.

---

## 8. Explicitly not claimed

- Comment posting has **never** succeeded in a test. See R1.
- LLM synthesis **is** verified: both roles reach `COMPLETED` with one HIGH finding and zero
  RLS errors. See R2.
- Comment posting still never succeeds (R1). Everything before it — rules, AI synthesis,
  blast-radius scoring, finding persistence, both provenance attestations — is verified
  under `scandrix_runtime`.
- 19 live repositories (150 bare-pool calls) still reach RLS tables without setting either
  GUC. Under `scandrix_runtime` they read zero rows and their writes are rejected, so
  any feature behind them is degraded today. This is a real enforcement gap, inventoried
  and ratcheted by `rls_bypass_test.go`, and NOT fixed. See R3.
- `app.is_system_worker` remains a broad bypass across all 43 RLS tables. This is the largest
  remaining security weakness in the model, and R3 describes the fix.
