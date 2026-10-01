# ScanDrix API Reference (Enterprise Surface)

Base paths: `/api/v1/*` and mirrored at root `/*` (Next.js proxy compatibility). Auth: `Authorization: Bearer <JWT>` unless noted. Errors are JSON `{"error":"..."}` with 401 (missing/invalid auth or workspace context), 403 (RBAC or entitlement), 404/409/429 where noted. All authenticated routes are workspace-scoped; cross-workspace IDs return 403, never data.

Legend: **[IMPLEMENTED]** works today · **[STUB]** endpoint exists, returns placeholder · **[SPECCED]** contracted, not built. STUB/SPECCED rows must not be sold as working.

## Health & probes (public)

| Method & path | Auth | Notes |
|---|---|---|
| `GET /healthz` | None | DB-backed health |
| `GET /livez` | None | Liveness |

## Auth & SSO (mixed)

| Method & path | Auth | Notes |
|---|---|---|
| `/auth/*` | Mixed | Login, register, refresh, OAuth, CLI device/loopback flows (see `AUTHENTICATION_WORKFLOWS.md`) |
| `GET /sso/check`, `GET /auth/sso/check` | None (rate-limited) | Domain → organization lookup |
| `GET /sso/login/{organizationId}` | None | SAML AuthnRequest initiation |
| `POST /sso/saml/callback/{organizationId}` | None | SAML ACS, signature-verified |
| SP metadata + OIDC callback | None | Per `auth_sso_controller.go`; OIDC shares session/provisioning path |

## Enterprise (feature-gated + RBAC)

| Method & path | Gate | Notes |
|---|---|---|
| `/scim/v2/*` (Users, Groups, Schemas, ServiceProviderConfig) | `FEATURE_SCIM_PROVISIONING` + constant-time bearer (`SCIM_BEARER_TOKEN`) | Filter `userName eq`, `startIndex`/`count` (cap 100). Deprovisioning revokes refresh tokens + suspends. Seat-quota 409 **[IMPLEMENTED]** — wired via `scim.BindTenant` in `cmd/api` & `cmd/server`. |
| `/sso-config` (`GET`, `POST /`, `POST /test-connection`, `GET /test-connection/{id}`, `POST /verify-domain`) | `FEATURE_SSO_SAML` | **[IMPLEMENTED]** Backed by `SSOConfigRepository` and PostgreSQL table `sso_configs` (migration 031) with per-workspace persistence. |
| `/workspaces/{workspaceId}/audit-logs` (`GET /`, `GET /export?format=cef\|syslog\|json`) | `update workspace` RBAC + `FEATURE_AUDIT_WAREHOUSE` | List is limit-capped (≤500); export streams CEF/syslog/JSON. |
| `/license/*` | `manage billing` RBAC | Activation, status, seats. Resolves via the shared `Resolver` (signed > plan-row > community). |
| `/billing/*` (protected) + `/webhooks/billing`, `/billing/webhook`, `GET /billing/plans` | Mixed | Razorpay lifecycle; webhooks HMAC-verified. |
| `/capabilities` | JWT | Single entitlement snapshot for the caller workspace (same Resolver — cannot disagree with gates). |

## Review pipeline

| Method & path | Notes |
|---|---|
| `/reviews/*` | Review CRUD, status, approvals |
| `/pull-request-messages/*` | Comment templates + suppression rules |
| `/findings/*`, `/issues/*` | Findings feed, issue tracking + auto-tickets |
| `/rule-like` | Rule feedback signal |
| `/webhooks/{github,gitlab,bitbucket,azure-repos,azure,forgejo,ingest}` + `/github|/gitlab|/bitbucket|/azure-repos|/forgejo/webhook` | HMAC-verified ingestion; invalid → 401 drop; dedup via `repo:pr:sha` + Redis lock → 200 ack on duplicates |
| `/agent/*`, `/workflow-queue/*` | Agent runs, queue introspection |

## Rules & policy

| Method & path | Gate | Notes |
|---|---|---|
| `/rules`, `/drixy-rules`, `/automations` | `manage rules` RBAC | Rule lifecycle, dry-run, automations |
| `/parameters`, `/organization-parameters` | JWT | Workspace/org settings, BYOK providers, model overrides, spend limits |
| `/config/centralized`, `/cli/config/*` | JWT | Centralized config tree + PR service |

## Teams & integrations

| Method & path | Gate | Notes |
|---|---|---|
| `/teams`, `/team-members` | `manage members` RBAC | Teams, invites, role assignment (canonical roles), per-repo allowlists, team CLI keys (`scandrix_` prefix) |
| `/organization`, `/workspaces` | `update workspace` (workspaces) | Org profile, workspace management |
| `/integrations`, `/integration`, `/integration-config` | `manage integrations` RBAC | SCM + PM connections |
| `/repos`, `/code-management` | JWT | Tracked repositories, per-repo settings |
| `/permissions` | JWT | Caller permission introspection |

## Analytics & cockpit

| Method & path | Notes |
|---|---|
| `/usage`, `/spend-limit` | Token usage, budgets, alerts |
| `/cockpit`, `/code-health`, `/productivity` | DORA + quality rollups — reads `materialized_dora_daily_rollups` (IMPLEMENTED table, migration 038) |
| `/pull-requests` | PR dashboard + review studio data |

## CLI & misc

| Method & path | Notes |
|---|---|
| `/cli/*` (authorize, validate-key, login flows, reviews, trials, sessions, drixy-rules, business-validation) | CLI auth (loopback + device flow), CLI review execution, trial gating |
| `/mcp` | MCP streamable HTTP server |
| `/skills` | Agent skills surface |
| `/user`, `/user-log` | Profile, user activity log |
| `/notifications` | Notification channels |
| `/health` | Webhook delivery health + DLQ retry |

## Conventions

- **Pagination:** SCIM uses `startIndex`/`count` (cap 100); audit logs use `limit` (cap 500). New list endpoints must follow one of the two and document which.
- **Rate limiting:** Redis token-bucket globally + tier-aware limiter on authenticated routes; CLI/auth endpoints sit behind a stricter limiter group (`authLim`). Exact quotas and headers are defined in `internal/api/middleware` + `internal/cache/limiter` — this doc must quote them verbatim when freezing the contract (SPECCED item).
- **Idempotency:** Webhook → `repo:pr:sha` inbox dedup; billing webhooks carry idempotency keys (`idempotency_store.go`).
- **Versioning:** Breaking changes require a new dated migration-style contract note here; additive fields are backward compatible by default.
