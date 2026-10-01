# ScanDrix — Security Remediation Implementation Plan

**Companion to:** `AUDIT_REMEDIATION.md`
**Date:** 2026-09-29

---

## Principles this plan follows

These are the constraints the user set. They are binding, not aspirational.

1. **No secrets in code.** Every new configuration value goes in `.env` (local) / secret manager (staging, prod), and gets a name-only entry in `.env.example`. Never a literal default that could be mistaken for a real value.
2. **Fail closed.** If a required dependency is missing, return an error or refuse to start. Never silently degrade to a permissive path.
3. **One code path per concern.** If two call sites do the same security check, that is a bug waiting to happen (it already happened in F-03). Extract and share.
4. **Absence is reported, never substituted.** Per AGENTS.md §2.7. A metric is computed from real data or returned as `null` with a reason in `unavailable`. Never a constant.
5. **Blast radius check before every edit.** Before changing a function, grep every caller. Record the count. Re-grep after.
6. **Every fix ships with a test** that fails before the fix and passes after.
7. **No behavioural regression.** Existing tests must stay green. If a test must change, the change is justified in the commit message.

---

## Verification gate (every phase)

No phase is complete until all four pass:

```bash
go build ./...                      # exit 0
go vet ./...                        # exit 0
go test ./internal/api/... ./internal/auth/... ./internal/organization/...
go test ./...                       # must remain at 294 ok / 0 fail (or higher)
```

Plus, for phases touching HTTP surface, a live re-test against the running stack.

---

## Phase 1 — Account takeover (P0)

**Goal: an unauthenticated caller can no longer reach any account's data or tokens.**
These three are ordered because F-01 and F-02 are pure deletions (zero regression risk), and F-03 is a refactor of shared logic.

### 1.1 F-01 — Delete `POST /auth/oauth`

**Blast radius:** `grep -rn "handlePostOAuth\|PostOAuthRequest" --include=*.go .` → **1 reference**, the route registration itself.

- [ ] Delete the route registration at `internal/api/controllers/auth_controller.go:320`.
- [ ] Delete `handlePostOAuth` and `PostOAuthRequest` from `auth_oauth_controller.go`.
- [ ] Confirm no other file references them (re-grep must return nothing outside the deleted lines).
- [ ] **Test:** new test asserts `POST /api/v1/auth/oauth` → 404/405 for any caller, authenticated or not.

**Note on intent:** this endpoint mints tokens for an account found by a *client-supplied* email. That is unrecoverable by configuration — the only safe fix is removal. The legitimate flow is `GET /auth/oauth/{provider}/authorize` → provider redirect → `GET /auth/oauth/{provider}/callback`, which derives identity from the provider.

**Edge cases to verify before deleting:**
- Does the CLI or any client call `/auth/oauth`? → grep the CLI (`internal/cli`) and any docs.
- Does the E2E test suite hit it? → `grep -rn "auth/oauth" test/ scripts/ site/`.

### 1.2 F-02 — Authenticate `/user/*`, remove body-sourced identity

**Blast radius:** `router.go:608` mount; 4 handlers in `user_controller.go`; `JoinOrganizationUseCase` has 3 production references and 1 test.

- [ ] Add `pr.Use(c.authService.Middleware)` — `UserController` already receives `authCtrl`, so no constructor change is needed if the method is exposed; otherwise add an exported accessor. **Prefer adding an exported `AuthMiddleware()` on `AuthController`** over changing the constructor signature, to avoid touching `router.go:470` and the module wiring.
- [ ] `handleJoinOrganization`: delete the `req.UserID` branch and the `uuid.NewSHA1(..., "default-user")` fallback. Derive the actor only from `auth.AccountProfileFromContext`. If absent → 401.
- [ ] Require and validate `InvitationCode` inside `JoinOrganizationUseCase.Execute` before any write. The field is already parsed at `user_controller.go:187` and silently dropped — wire it through.
- [ ] `handleUpdateTargetUser` (`PATCH /{targetUserId}`): add an ownership check — the caller may only update themselves, or must hold an admin role. Currently no-op but shaped to invite a future unguarded write.
- [ ] Move `GET /user/email` and `GET /user/invite` behind auth as well (both are account-state oracles).
- [ ] **Tests:** (a) unauthenticated `POST /user/join-organization` → 401; (b) authenticated caller cannot pass another `userId` in the body; (c) missing/invalid invitation code is rejected; (d) `/user/info` still works for an authenticated caller (regression guard).

### 1.3 F-03 — Single shared credential authentication path

**This is the highest-value change in the plan.** Today `handleLogin` and `HandleCLIAuthorizeApprove` each implement credential verification independently; only the first has lockout.

**Blast radius:** 2 `VerifyPassword` call sites in auth controllers (`auth_controller.go:400`, `auth_cli_controller.go:1093`). `internal/core/crypto/crypto_service.go:213` is an unrelated legacy service — leave it.

**Design:** add to `auth_security.go`:

```go
// CredentialAuthStatus enumerates why a credential check ended the way it did.
type CredentialAuthStatus int

const (
    CredentialAuthOK CredentialAuthStatus = iota
    CredentialAuthInvalid
    CredentialAuthLocked
    CredentialAuthRateLimited
)

// CredentialAuthResult is the single, complete outcome of a credential check.
type CredentialAuthResult struct {
    Status     CredentialAuthStatus
    User       *database.UserRecord
    RetryAfter time.Duration
}

// AuthenticateCredentials is the ONLY supported way to verify an email/password
// pair. It performs, in order:
//   1. account lockout check      (per-account, distributed via Redis when available)
//   2. per-account rate limiting  (defeats distributed proxy blasting)
//   3. password verification      (with a real KDF dummy-verify on miss, so an
//                                  unknown email costs the same as a wrong password)
//   4. failure accounting         (increments the counter; may trigger lockout)
//   5. success bookkeeping        (clears the counter; transparently upgrades the
//                                  stored hash if the cost factor is stale)
//
// Every caller must branch on Status and translate it to an HTTP response.
// No caller may call auth.VerifyPassword directly — that bypasses steps 1, 2 and 4.
func (c *AuthController) AuthenticateCredentials(
    ctx context.Context, email, password string,
) CredentialAuthResult
```

**Invariants the implementation must hold:**
- Steps 1–4 run in exactly that order on every call, including when `c.repo == nil`.
- When the user does not exist, `DummyVerify` still runs (timing parity).
- When the user exists but the password is wrong, the real `VerifyPassword` runs. Both paths must cost approximately the same — do **not** short-circuit one.
- On success, `NeedsRehash` upgrade runs before returning.
- Telemetry (`AuthAttemptsTotal`) is emitted exactly once per call, from inside this function — not from the callers. This prevents the two paths from drifting again.

**Call-site changes:**
- [ ] `handleLogin` (`auth_controller.go:354-445`) → delegate. Preserve the exact existing status codes and messages so no client breaks: 429 for locked, 429 for rate-limited, 401 for invalid.
- [ ] `HandleCLIAuthorizeApprove` (`auth_cli_controller.go:1089-1115`) → delegate. Must gain lockout (this is the fix).
- [ ] Keep the post-auth checks (status active, workspace assigned) in the callers — they are not part of credential authentication.
- [ ] **Tests:** (a) table test over both entry points asserting identical status for: unknown user, wrong password, correct password, locked account, rate-limited account; (b) the CLI path now returns 429 after `MaxFailedLoginAttempts`; (c) dummy-verify is invoked on unknown user (assert via a test seam or timing-free counter).

### 1.4 F-04 — SMTP fails closed

**Blast radius:** `mailer.NewSender` has 3 production callers (`cmd/api`, `cmd/server`, `cmd/worker`).

- [ ] `NewSender` returns a sender whose methods return `ErrSMTPNotConfigured` when `Host` is empty, instead of `NewNoopSender()`.
- [ ] Add `ErrSMTPNotConfigured` to `mailer.go`.
- [ ] Update the 3 `main.go` callers to surface a clear startup warning in development and a hard failure in production.
- [ ] `cmd/envcheck`: add `SMTP_HOST` as required for non-dev. (`envcheck` currently checks 10 of ~400 vars — see F-76; do the minimal correct thing here, the full fix is Phase 5.)
- [ ] **Tests:** `NewSender` with empty host → error; with host → SMTP sender; NoopSender retained for tests but marked test-only in its doc comment.

---

## Phase 2 — Honest data (P0/P1)

**Goal: no handler invents data, and no metric substitutes a constant for an absent value.**

### 2.1 F-07 — Cockpit pass rate

**Reference implementation to copy:** `internal/database/dora_repository.go:154-224`. Do not invent a new pattern.

- [ ] `models.CockpitMetrics.PassRatePercentage` → `*float64`.
- [ ] Add `Unavailable []string` to the model with the documented reason codes.
- [ ] `audit_repository.go:250-255`: delete the `PassRatePercentage: 100.0` nil-repo branch; return `nil, error`.
- [ ] `audit_repository.go:263-266`: `WHEN COUNT(DISTINCT r.id) = 0 THEN NULL`.
- [ ] `workspace_controller.go:112-119`: delete the `100.0` initialiser; on repo error return `503`.
- [ ] Update the DTO mapper and every consumer (including any TS/JS client) to handle `null`.
- [ ] **Tests:** empty workspace → `pass_rate_percentage: null` + `unavailable: ["no_data_source"]`; repo error → 503; populated workspace → real computed value.

### 2.2 F-39/F-40/F-41 — Sandbox and dead analytics

- [ ] `internal/sandbox/null/null_provider.go`: `Grep` returns `("", nil)` plus an `unavailable` reason rather than `"No matches found."`.
- [ ] `internal/sandbox/e2b/provider.go:327-343`: fail closed on remote-execution error unless `ALLOW_UNSANDBOXED_COMMAND_EXECUTION=true` (env var, documented in `.env.example`). Return a non-zero `ExitCode` with an error.
- [ ] Delete `internal/analytics/dora/calculator.go` — fabricated literals, superseded.
- [ ] Delete the three unreferenced `*_inmemory*.go` cockpit services.
- [ ] **Verify before deleting:** re-grep each constructor for non-test callers. If any has one, port it to the `dora_repository.go` pattern instead of deleting.

### 2.3 F-10/F-42/F-43 — Fabricated success responses

- [ ] `organization_parameters_controller.go:179-191` → `503`, not a fabricated record.
- [ ] `organization_parameters_controller.go:406-412` → `503` on delete failure. **This is the security-relevant one: a BYOK credential reported as revoked while still live.**
- [ ] `organization_parameters_controller.go:751-761` → add `SupportsVision` to `kernel.ModelCapabilities`, populate per provider, `null` when unknown. Remove the hardcoded `true`.
- [ ] `organization_parameters_controller.go:769-786` → distinguish "no overrides" from "query failed".
- [ ] `audit_controller.go:72-77` and `:113-116` → `503`. **`:113` is the SIEM export — returning 200 + empty body tells a SOC "zero security events." That is the single most dangerous instance in this phase.**
- [ ] `system_controller.go:267-289` → default to `"unknown"` / actual `APP_ENV`, never `"production"`.
- [ ] **Tests:** for each, assert 503 when the repo is nil and that no response body contains a freshly generated UUID.

---

## Phase 3 — Availability + authorization (P1)

### 3.1 F-08/F-09/F-64 — Health endpoints

- [ ] `docker/webhooks.Dockerfile` (or wherever the webhooks HEALTHCHECK is defined) → probe `:8081/healthz`.
- [ ] **Audit every service the same way:** for each container, read its actual listen port and its healthcheck target. Add a test or script that asserts they match, so this cannot regress.
- [ ] `infra/terraform/alb.tf:29` → `/healthz`.
- [ ] `docker/caddy/Caddyfile:19` → `/healthz`.
- [ ] Add `health_check` blocks to `ecs.tf` task definitions → `/healthz` on all three.
- [ ] Add `HEALTHCHECK` to `docker/{api,worker,webhooks}.Dockerfile`.
- [ ] **Verify live:** `curl -sf localhost:8080/healthz` → 200; `docker ps` shows all containers healthy.

### 3.2 F-15 — Authorization boundaries

Apply the same discipline to each: identify the correct existing guard, apply it, add a test proving the boundary.

- [ ] **F-15a** `router.go:677` → wrap `/permissions` in `rbac.RequirePolicy(policyEngine, rbac.ActionManage, rbac.ResourceMembers)`, matching `/teams` at `:690`. Also add a role assertion inside `AssignRepositories` so the service layer is safe regardless of routing.
- [ ] **F-15b** `agent_controller.go:78-86` and `:135-140` → ignore the body `OrganizationID`; set from context unconditionally; reject a mismatching body value with 403.
- [ ] **F-15c** `cli_reviews_controller.go:84-101` → add `GetCliReviewByID(ctx, wsID, id)`; return 404 on org mismatch.
- [ ] **F-15d** `cli_review_controller.go:231-256` → add the `c.authenticate(r)` call that `handleReview` already has; verify the resolved org matches the job record.
- [ ] **F-15e** `router.go:633-634` → wrap in `RequirePolicy`.
- [x] **F-15f** MCP → default **disabled**; requires a real session; the organisation is taken from the authenticated context, never from the request body.
- [ ] **Tests:** for each, a cross-tenant access test — authenticate as workspace A, request a resource owned by workspace B, assert 403/404 (never 200).

---

## Phase 4 — CI/CD + dead code (P0/P2)

### 4.1 F-05 — Fork PR OIDC

- [ ] `terraform-plan.yml`: `pull_request` job → `fmt` + `validate` only. Remove `id-token: write` from that path.
- [ ] Move `plan` to a separately gated trigger with no checkout of PR-controlled code.
- [ ] **Test:** open a throwaway PR adding a `data "external"` block and confirm the job does not receive AWS credentials.

### 4.2 F-06 — Committed DB password

- [ ] Treat `scandrix_secure_pass` as compromised. Rotate it.
- [ ] Move role creation out of `migrations/015` into `migrations/ops/` (model on `002_least_privilege_runtime_role.sql`, which already creates the role with no password).
- [ ] Mirror the change in `internal/core/repositories/migrations/m015_*.go`.
- [ ] Add the literal to the secret-scan denylist.
- [ ] Plan the history rewrite separately — it is disruptive and should be its own reviewed change.

### 4.3 Delete the dead landmines

Delete outright; do not repair. Each is unreferenced and each is one wiring mistake from a live vulnerability.

- [ ] `internal/shared/infrastructure/core.go:130-162` — `AuthGuard` accepts any `Bearer` token.
- [ ] `internal/identity/infrastructure/security_services.go:70-75` — default JWT secrets.
- [ ] `internal/core/infrastructure/config/loaders.go:69-76` — default JWT secrets; refresh derived from access key.
- [ ] `internal/enterprise/audit/repository.go:51-54` — default HMAC secret.
- [ ] `internal/common/email/service.go` — zero importers.
- [ ] `internal/auth/sso.go` — `SSOService`, no production callers, contains the unsigned-SAML fallback.
- [ ] **Re-grep every symbol before deleting.** If a deleted file is the only implementation of something referenced, port that first.

---

## Phase 5 — Config surface (P2)

- [ ] Populate `.env.example` with every variable in the AUDIT_REMEDIATION gap list (auth, transport, secrets, hashing tunables).
- [ ] Fix the API contract inconsistency: `POST /auth/refresh` takes `refresh_token` but login returns `refreshToken`.
- [ ] Add `.dockerignore`.
- [ ] Add `*.tfvars.example`.
- [ ] Improve `cmd/envcheck` beyond the SMTP entry from 1.4.
- [ ] Add a schema-drift test (F-34): parse `migrations/*.sql`, assert every table referenced in Go exists.
- [ ] Add a RLS migration for the 4 unprotected tables (F-37).

---

## Risk register

| Change | Risk | Mitigation |
|---|---|---|
| Delete `POST /auth/oauth` (1.1) | A client depends on it | Grep CLI/docs/tests first. The legitimate OAuth flow is unaffected. |
| Auth `/user/*` (1.2) | Signup/onboarding flow breaks if it relied on the public route | Check the web UI's onboarding calls. Test the full register → verify → join flow. |
| `AuthenticateCredentials` refactor (1.3) | Status codes drift; lockout messages change | Preserve exact codes/messages. Table test asserting parity across both entry points. |
| `CockpitMetrics` pointer field (2.1) | Client breaks on `null` | Update DTO + client together. Never revert to a `0` substitute — that is the original bug. |
| Deleting dead code (4.3) | A file is actually referenced | Re-grep each symbol immediately before deleting. |
| ALB `/healthz` (3.1) | `/healthz` may not carry the readiness signal `/health` did | Verify what `/healthz` actually checks before switching. Prefer adding an unauthenticated `/health` alias if the checks differ. |

---

## Definition of done

- [ ] `go build ./...` exit 0
- [ ] `go vet ./...` exit 0
- [ ] Full test suite green (baseline 294 ok / 0 fail)
- [ ] Every phase has a test that **failed before** its fix
- [ ] Live re-verification: the F-01/F-02/F-03 attacks from `AUDIT_REMEDIATION.md` all now return 401/403
- [ ] All containers healthy
- [ ] No secret added to any tracked file; `.env.example` updated by name only
- [ ] `AUDIT_REMEDIATION.md` findings marked resolved with the commit SHA

---

## Progress log

**Verification at last update:** `go build ./...` exit 0 · `go vet ./...` exit 0 · **294 packages pass, 0 fail** (matches the pre-change baseline) · all 5 containers healthy.

Every entry marked "live-verified" was re-attacked against the rebuilt running stack and now returns the corrected response.

| Phase | Finding | Status | Evidence |
|---|---|---|---|
| 1.1 | F-01 `POST /auth/oauth` takeover | **DONE** | live-verified: 404 (was: issued a victim session) |
| 1.2 | F-02 `/user/*` unauthenticated | **DONE** | live-verified: 401 (was: 400 + business logic ran) |
| 1.3 | F-03 CLI lockout bypass | **DONE** | live-verified: 429 (was: 200 + owner token) |
| 1.4 | F-04 SMTP silent discard | **DONE** | 7 mailer tests; `SMTP_HOST` required in prod config |
| 2.1 | F-07 fabricated 100% pass rate | **DONE** | live-verified: `"pass_rate_percentage":null` + `unavailable` |
| 2.2 | F-39 sandbox `Grep`/`Run` | **DONE** | null provider now errors instead of "No matches found." / exit 0 |
| 2.3 | F-10/F-42/F-43 fabricated responses | **DONE** | live-verified: unknown key → 404; `environment:development`, `commit:unknown` |
| 3.1 | F-08/F-09 health endpoints | **DONE** | live-verified: webhooks healthy, streak 0 (was 132); ALB+Caddy → `/healthz` |
| 3.2 | F-15 authz boundaries | **PARTIAL** | done: 15a, 15b, 15d, 15e. **not done: 15c, 15f** |
| 4.1 | F-05 fork PR → AWS OIDC | **DONE** | `pull_request` now fmt+validate only, no `id-token` |
| 4.2 | F-06 committed DB password | **DONE** | password now read from `scandrix.app_password`; **rotate the old value** |
| 4.3 | dead auth landmines | **DONE** | 4 hardcoded secrets removed; `AuthGuard.Middleware` deleted |
| 5 | `.env.example` | **DONE** | 26 undocumented security variables added, names only |

### Bugs found and fixed while implementing

These were not in the original audit; they were surfaced by the fixes and would have been regressions if left.

- **`cleanUpOrphanedWorkspace` failed open.** `if err == nil && len(remainingUsers) > 0` meant a DB error skipped the early return and fell through to the **deletes**. Same pattern on the org-row deletion. Now fails closed on every error (`join_organization.go`).
- **Duplicate `/user/email` route.** `router.go:565` registered it publicly, shadowing the `UserController` mount at `:608`. Authenticated registration in the controller would have been dead code. Deduplicated, rate limiter moved beside the route.
- **`router.go:565` removal dropped rate limiting** on `/user/email` and `/user/invite`. Caught during implementation; the limiter now lives in the controller.

### Honest limitations of what was done

- **F-15c not done** — `GetCliReviewByID` still has no organisation filter. Left alone because it needs a new repository signature and a caller audit; it is a read-only cross-tenant disclosure, not a write.
- **F-15d is only half fixed** — `handleGetJobStatus` now requires authentication, but `try.JobStatusResponse` carries no owning organisation, so an *authenticated* caller of tenant B can still read tenant A's job if they learn the id. Closing it needs the job store to record the organisation at creation. Marked in the code.
- **F-15f now done.** `IsMcpServerEnabled` defaults to off, the MCP transport requires a real session (a nil authenticator fails closed with 503), and the workspace comes from the authenticated context rather than the request body. A build-tag-gated test asserts the disabled-by-default contract.
- **F-12 now done** — refresh tokens are stored as a SHA-256 digest in `auth.tokenHash`; the plaintext column was dropped and forced RLS applied. Live-verified that the digest matches, rotation returns 200, and replay returns 401. Note this covers the `auth` table only; device/CLI session persistence and `cli_auth_sessions` are still open.
- **F-11 now done** — a domain can no longer be claimed without proof. The instant auto-approval that fired whenever `cloudMode` was false (and it was hardcoded false, so it applied everywhere) is gone; ownership requires a real DNS TXT challenge or a confirmed token. `/sso/domains/verify-dns` and `/confirm-token` moved behind authentication and no longer trust a body-supplied `workspace_id`. `cloudMode` is now read from `SCANDRIX_CLOUD_MODE`/`API_CLOUD_MODE`.
- **F-11 exposed a real latent bug.** Because `VerifyDNS` used to return early on an already-verified record, it never performed a lookup, so quoted and segmented TXT values (which real DNS providers return) were never handled. With the bypass removed, a correctly published but quoted record failed to verify. Fixed in `normalizeTXTValue` and covered by a table test.
- **F-13 now done** — public CLI self-registration is closed by default and requires an explicit `ALLOW_PUBLIC_CLI_REGISTRATION=true`. The policy lives in `cliPublicRegistrationAllowed()` and is unit-tested (9 cases). Live-verified: unauthenticated `POST /cli/authorize/approve` with `action=register` returns 403 and creates zero owner rows.
- **F-51 now done** — added `.dockerignore`. The `COPY . .` in all three Dockerfiles was carrying `.env`, a 1.5GB `.git`, a 452MB `bin/` and terraform state into the build context. Context measured 2724 MB -> 366 kB. Note the non-obvious part: `*` does not cross directory separators in `.dockerignore`, so the initial `*.tfstate` rule silently did nothing for `infra/terraform/`; the patterns must be `**/*.tfstate`. Verified with canaries and a clean `--target builder` build.
- **F-11 remaining limitation** — verification records still live in a process-local map, so a restart drops pending challenges and a multi-replica deployment would not share them. Acceptable for a single self-hosted instance; it needs a table before horizontal scaling.
- **`/user/email` remains a public account-existence oracle.** Authenticating it would break the signup flow, and there is no invitation store to replace it with. Documented in the code as an accepted trade-off with the removal condition.
- **Committed secrets remain in git history.** Deleting the literals does not remove them. `scandrix_secure_pass` must be rotated; a history rewrite is a separate, disruptive change.
- **5 existing tests were updated** because they asserted the vulnerable behaviour (`join-organization` returning 200 unauthenticated, `AuthGuard` accepting `Bearer valid-token-123`, cockpit returning 200 with a nil repo, audit export returning 200, `NoopSender` from `NewSender`). Each replacement asserts the corrected contract, and the rationale is in a comment at each site.

---

## Batch: F-49, F-44, F-52/53, F-18

### F-49 — invite lookup scoped to the caller

Authenticating the route was necessary but **not sufficient**: the handler read
`?userId=` from the query, so any authenticated tenant could still read another
account's invitation status. The parameter is now ignored and the handler reports
`profile.ID`.

The regression test asserts the response `userId` equals the *caller's* id while
passing a different id in the query. A test that only checked `valid`/`status`
would pass even with the scoping still wrong, because the fake repository
returns the same row regardless of the id it was asked for. Tests also use a real
signed token rather than an injected context, so the actual middleware runs.

### F-44 — GitHub account identity

Route moved to the authenticated group; `GITHUB_USER` and the live
`api.github.com/user` fallback both removed; resolution is now workspace-scoped
to the caller's own integration record.

Not done: `docker-compose.cluster.yml` still carries the older broker config and
needs the same treatment before that file is used for a real deployment.

### F-52 / F-53 — RabbitMQ credentials

The committed `default_user`/`default_pass` in `docker/rabbitmq/rabbitmq.conf`
were not merely a leaked secret: because RabbitMQ only honours the
`RABBITMQ_DEFAULT_*` environment variables when the config file omits them, they
silently defeated compose's `${RABBITMQ_PASSWORD:?...}` guard. The `:?` check
passed while the broker used the repository's password.

Management UI and AMQP are now bound to `127.0.0.1` instead of `0.0.0.0`.

### F-18 — access-token revocation (the largest change in this batch)

Logout only stopped *renewal*. A copied bearer token kept working for its full
15 minutes, and the middleware's `revocationChecker` was never injected, so the
protection existed only as code that read as if it were enforced.

Added `migrations/041_revoked_access_tokens.sql` and
`internal/database/token_revocation_repository.go`. A row is a **cutoff**, not a
token: any access token whose `iat` is `<=` the cutoff is rejected. One insert
per logout invalidates every outstanding session token without enumerating them,
and tokens issued after the cutoff survive so a refresh racing with logout is a
new session.

The lookup runs before any tenant context exists, so RLS is forced to
`app.is_system_worker` only and every access goes through `ExecAsSystem`.

**Fail-closed, deliberately.** A store error rejects the request rather than
answering "not revoked", and logout returns `503` when it cannot spend the
refresh token *and* record the revocation.

**A fail-open path was found while implementing this.** Logout with a nil
repository returned `200 "logged out successfully"` while revoking nothing.
Implementing this exposed it, so logout now clears cookies on every path but
only returns `200` when the session was genuinely ended. Two existing tests
asserted the old `200`; both were updated with the rationale at the site.

Live verified: `/me` `200` → `logout` `200` → same token `/me` **`401
{"error":"unauthorized: session has been revoked"}`**.

### Honest limitations

- **Revocation adds a per-request database dependency.** One indexed lookup on
  every authenticated request, on a path that previously touched no store. The
  table is small and short-lived enough to move to Redis later if the latency
  matters, with identical semantics.
- **`iat` is second-granular**, so a token minted in the same second as the logout
  shares the cutoff and is revoked. Conservative and intentional, but it is a
  real boundary rather than an exact one.
- **`accessTokenRevocationRetention` duplicates `auth.DefaultAccessTokenTTL`** to
  avoid taking a dependency from `internal/database` back to `internal/auth`. A
  test fails if they drift, because a shorter retention would purge rows while the
  tokens they cover are still valid.

## Batch 2 — F-63, F-26, F-31, F-15d, plus a new fabricated-response finding

### F-63 — Appwrite is optional; it should never have blocked boot

Re-checked the original claim instead of taking it at face value, and it was
half wrong: the credentials were in `.env` all along. The real defects were that
Compose never forwarded them, and that they were a hard production requirement.

Appwrite is optional *by construction* — nil-safe client, `slog.Warn` on upload
failure — so requiring it crash-looped the whole production API over an archive
feature. Now a warning. `JWT_SECRET` / `DATABASE_URL` / `SMTP_HOST` stay hard
requirements, each pinned by a test, because the SMTP case really does lock users
out of their accounts.

### F-26 — CSRF exemption is now an allow-list, not a prefix

`strings.HasPrefix(path, "/cli")` exempted every `/cli` route. Replaced with the
three device-flow *start* endpoints, which must stay exempt because the browser
is being redirected away from. Everything else under `/cli` gets the standard
`Sec-Fetch-Site`/`Origin` check.

### F-31 — `user_code` is not a credential

It is short and human-typable, so it was an enumeration oracle plus a
`userAgent` leak. Only `state`/`device_code` are accepted now, and
`userAgent` is gone. **Compatibility risk:** the CLI lives in an external repo
that was polling this endpoint by `user_code`; if the shipped CLI does the same,
it now gets 400. Confirm against the CLI repo before release.

### New finding — `/cli/business-validation` fabricated a queued job

Returned `{"success":true,"status":"QUEUED"}` while doing nothing. Now 501 with an
explicit reason. **Needs a product decision:** implement it or stop the CLI
calling it.

### F-15d — tenant-scoped jobs, and the regression the earlier fix caused

`JobRecord.Input` already carries `OrganizationID` and `IsTrialMode`, so the
earlier note claiming ownership was unknowable was wrong. Split the handler:
authenticated mount requires an org match, public mount serves trial jobs only.
Both 404 on mismatch. Also reverted the functional regression my earlier F-15d fix
introduced, which had broken public PR trial polling.

### F-15c — deliberately left open

`GetCliReviewByID` has no org filter, but the store has no production writer, so
the endpoint always 404s. Fixing it needs a writer that does not exist. Recorded
as open rather than marked done on the strength of a 404.

### Honest limitations

- The F-31 change is a **behavioural API change** to an endpoint consumed by an
  external CLI I cannot see or test. That is the one item here most likely to
  need follow-up.
- Splitting the job-status handler means two code paths over one job table. The
  tests cover the matrix, but the boundary is easy to regress if someone later
  merges the handlers back.
- `/cli/business-validation` now returns 501. That is honest, and it is still a
  broken feature until someone decides its fate.
- F-62 (rotate the production `JWT_SECRET`) is untouched and still needs the
  operator's decision on timing; nothing in this batch changed that.

## F-27 device-session persistence (re-verified against live PostgreSQL)

The audit recorded F-27 as DONE for the *atomic consume* half only. Probing
the durable half against the live database showed the CLI device-flow session
store had **never persisted at all**:

- `cli_auth_sessions` carries two merged schemas. Migration 009 defines
  `id, session_id, user_id, workspace_id, status, token, expires_at,
  created_at`; `cli_session_store.ensureTable` defines the RFC 8628 columns.
  Migration 014 (`m014_cli_auth_sessions_alignment`) merged them with
  `ALTER TABLE`, so both sets are live.
- `session_id` is `NOT NULL` with **no default**. `CreateCLISession` never
  supplied it, so every INSERT failed with SQLSTATE 23502.
- `CreateSession` discarded that error with `_ =`, and `GetByDeviceCode`
  silently fell back to memory on any read error. The device flow therefore
  looked healthy in a single process while writing nothing durable. On restart
  or scale-out, every pending device login vanished.

Fixes:
- `CreateCLISession` now supplies `session_id`, bound to `device_code`, which is
  already `UNIQUE`, indexed, and the lookup key both writers use. The column
  could not be dropped because `identity/infrastructure/sql_repositories.go`
  still reads it.
- `GetCLISessionByDeviceCode` and `GetCLISessionByUserCode` scan the nullable
  `redirect_uri` through a `*string`; scanning it into a plain string failed on
  NULL.
- `CreateSession` logs a durable-write failure instead of discarding it. It
  still returns success, because the in-memory fallback did succeed and failing
  the request would be a behaviour change, not a fix.
- New `cli_session_lifecycle_test.go` runs against a live database: create →
  read back → complete → read back, plus a NULL-`redirect_uri` case. Verified
  red/green by reverting the fix and confirming both tests fail with the exact
  NOT NULL violation.
- Live table went from 2 rows to 12, all with `device_code` and `session_id`
  populated.

Two things deliberately **not** changed, because they are design decisions
rather than defects:

- **Four writers, four incompatible column sets** target `cli_auth_sessions`:
  `database/auth_repository.go` (RFC 8628 + tokens), `identity/infrastructure/
  sql_repositories.go` (`session_id`), `core/repositories/
  billing_license_repository.go`, and `core/repositories/cli_session_repository.go`.
- **`core/repositories/cli_session_repository.go:46` inserts into
  `session_code`, `token_payload`, `client_ip`, and `updated_at` — none of
  which exist in the table.** That path can never succeed. It has no callers, so
  it is currently dead code rather than a live bug, but it should be deleted or
  rewritten rather than left to rot next to a working path.

## Correction: F-62 was not an operator decision

The previous entry in this file says F-62 "needs the operator's decision on
timing". That was wrong. `terraform.tfstate` contains only LocalStack resources
(account `000000000000`, log groups `scandrix-local-*`), the placeholder was
never committed, and no production deployment exists — so there was no live key
to rotate and no mass logout to avoid. The committed literal is now a required
sensitive `jwt_secret` variable with length and placeholder validation, so the
first real `terraform apply` cannot proceed with a known signing key. The local
`.env` value is a real secret and needs no change.

Also: the RLS inventory guard (`RLS_TABLES`) had drifted by five tables, one of
which (`revoked_access_tokens`) was added by the F-18 migration without updating
the inventory. The guard only runs when `SCANDRIX_E2E_RUNTIME_DSN` is set, so it
never fired in the default suite run. All five are now listed.
