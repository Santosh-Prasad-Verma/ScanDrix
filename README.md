<div align="center">

<p align="center">
  <img src="./assets/ScanDrix_logo_img3.png" alt="ScanDrix" width="100%" />
</p>

**AI Code Review & Security Gate**

[![Go](https://img.shields.io/badge/Go-1.25.3-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Next.js](https://img.shields.io/badge/Next.js-14.2-000000?logo=next.js)](https://nextjs.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16%20%2B%20pgvector-4169E1?logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-proprietary%20v2.0-6E56CF)](#license)

</div>

---

ScanDrix reviews pull requests, gates commits, and reports what it actually found. It is a Go monorepo (`github.com/scandrix/backend`) shipping **11 binaries**, a **Next.js 14** dashboard, a terminal CLI, and a self-hosted agent-skill library.

```
 SCM webhook ──▶ webhooks service ──▶ Postgres outbox ──▶ RabbitMQ
                                                          │
                    ┌─────────────────────────────────────┘
                    ▼
              worker ──▶ review pipeline (16 stages) ──▶ LLM gateway ──▶ SCM publish
                                   │                              │
                                   └── attestation (in-toto/SLSA) ┘
```

---

## Table of Contents

- [What Is In This Repository](#what-is-in-this-repository)
- [Quick Start](#quick-start)
- [Repository Layout](#repository-layout)
- [The Binaries](#the-binaries)
- [How A Review Actually Runs](#how-a-review-actually-runs)
- [The Agent Harness](#the-agent-harness)
- [LLM Gateway & BYOK](#llm-gateway--byok)
- [SCM Providers](#scm-providers)
- [Database](#database)
- [Security Model](#security-model)
- [Dashboard](#dashboard)
- [The CLI](#the-cli)
- [Configuration](#configuration)
- [Infrastructure & CI](#infrastructure--ci)
- [Local Development](#local-development)
- [Testing](#testing)
- [Project Scale](#project-scale)
- [License](#license)

---

## What Is In This Repository

This is a **monorepo containing four shippable artifacts and their shared backend**:

| Artifact | Path | What it is |
|---|---|---|
| **Backend** | `cmd/*`, `internal/*` | 11 Go binaries — API, worker, webhook gateway, CLI, MCP manager, migrations, tooling |
| **Dashboard** | `scandrix-dashboard/` | Next.js 14 App Router admin UI (54 routes) |
| **CLI** | `cmd/cli`, `internal/cli` | The same Go binary — a Bubble Tea TUI, git hooks, and an MCP stdio server |
| **Marketing site** | `site/` | Static HTML/CSS/JS + `install` scripts |
| **Agent skills** | `skills/` | 23 SKILL.md bundles for Claude Code, Cursor, Codex |
| **Evals** | `evals/` | Offline, deterministic 11-domain evaluation harness |

---

## Quick Start

### 1. The CLI

```bash
# macOS / Linux
curl -fsSL https://get.scandrix.dev/install | sh

# or from source
make build                        # produces ./bin/scandrix-*
./bin/scandrix-cli auth login
./bin/scandrix-cli review --staged
./bin/scandrix-cli tui            # interactive cockpit
```

The installer resolves `linux`/`darwin` × `amd64`/`arm64`, falls back to `~/.local/bin` when `/usr/local/bin` is not writable, and runs `scandrix skills install` on completion to sync bundled agent skills into your editor.

### 2. The Full Stack

```bash
cp .env.example .env              # 398 variables — start with the required ones below
docker compose up -d              # postgres + rabbitmq + redis + migrate + api + webhooks + worker
curl localhost:8080/healthz
```

Minimal required variables:

```bash
DATABASE_URL=postgres://user:pass@localhost:5433/scandrix?sslmode=disable
JWT_SECRET=<32+ chars>
SMTP_HOST=smtp.example.com         # hard-required in production/staging
ANTHROPIC_API_KEY=sk-...           # or any one of the 20+ provider keys
```

In production and staging the API **refuses to boot** without `DATABASE_URL`, `JWT_SECRET`, and `SMTP_HOST`. Appwrite is deliberately only a warning — it is an optional artifact-archive feature and must not crash-loop the API.

---

## Repository Layout

```
.
├── cmd/                    # 11 binary entry points
├── internal/               # 49 packages
│   ├── review/             # ★ 288 files / 74k LOC — the review pipeline
│   ├── platform/           # 164 files / 57k LOC — SCM adapters (GitHub, GitLab, …)
│   ├── cli/                # 292 files / 49k LOC — terminal product
│   ├── api/                # 115 files / 34k LOC — chi HTTP router + controllers
│   ├── core/               # 158 files / 30k LOC — config, logging, queue, observability
│   ├── llm/                # 106 files / 16k LOC — gateway, providers, BYOK
│   ├── mcp/                #  58 files / 14k LOC — MCP server, client, manager
│   ├── rules/              # 114 files / 13k LOC — deterministic rule engine + Drixy
│   ├── agentharness/       #  34 files /  4k LOC — ★ the agent loop
│   ├── auth/, enterprise/, database/, sandbox/, notifications/ …
├── migrations/             # 48 SQL migrations + 1 ops script
├── docker/                 # per-service Dockerfiles, nginx, caddy, rabbitmq, pgvector
├── infra/                  # Terraform (AWS + Cloudflare) + CloudFormation wake-gateway
├── evals/                  # offline 11-domain eval harness (Go)
├── skills/                 # 23 agent skills
├── test/                   # integration, benchmark, matrix, mcp_manager suites
├── hunk-extension/         # ScanDrix findings panel for the Hunk diff viewer
├── site/                   # marketing site + install scripts
└── scandrix-dashboard/     # Next.js 14 dashboard
```

**★ = load-bearing.** `internal/review` and `internal/agentharness` are where the product actually lives.

---

## The Binaries

`make build` compiles all eleven into `./bin/`:

| Binary | Source | Port | Role |
|---|---|---|---|
| `scandrix-api` | `cmd/api` | 8080 | HTTP API — review, rules, orgs, billing, analytics |
| `scandrix-server` | `cmd/server` | 8080/8081/9090 | Combined server (Docker default entrypoint) |
| `scandrix-webhooks` | `cmd/webhooks` | 8081 | Webhook signature verification + outbox ingestion |
| `scandrix-worker` | `cmd/worker` | 8082 | Async review consumer, cron scheduler, outbox relay |
| `scandrix-cli` | `cmd/cli` | — | Terminal product (TUI, hooks, MCP stdio) |
| `scandrix-mcp-manager` | `cmd/mcp-manager` | 3101 | MCP connection CRUD, catalog, OAuth |
| `scandrix-migrate` | `cmd/migrate` | — | Idempotent SQL migrations |
| `scandrix-ast-cli` | `cmd/ast-cli` | — | Standalone AST/complexity analysis |
| `scandrix-analytics-cli` | `cmd/analytics-cli` | — | Usage and DORA reporting |
| `scandrix-try` | `cmd/try` | — | Trial-mode harness |
| `scandrix-keygen` | `cmd/scandrix-keygen` | — | Ed25519 license keypair generation |

### Service roles

The **worker** is split by `WORKER_ROLE`:

- `code_review` — RabbitMQ consumers, outbox relay, sandbox reaper
- `analytics` — DORA rollups, seat pruning, spend-limit alerts, payment reconciliation, telemetry beacon
- `all` (default) — both

Tier-aware timeouts are enforced per review: Community 15 min → Team 30 min → Enterprise 60 min.

---

## How A Review Actually Runs

### The canonical pipeline

`internal/review/pipeline/strategy.go` defines **16 stages**, executed sequentially by `PipelineEngine.Execute`. The first error aborts.

| # | Stage | Does |
|---|---|---|
| 1 | `DeepValidatePrerequisitesStage` | Closed/merged PR, locked PR, ignored title keywords, bot-author seat check, license entitlement |
| 2 | `DeepValidateNewCommitsStage` | Detect new pushes mid-review |
| 3–4 | `DeepResolveConfigStage` → `DeepValidateConfigStage` | Merge repo `.scandrix.yml` with org/team params |
| 5 | `DeepFetchChangedFilesStage` | Diff + per-file content (caps: 350 legacy / 2000 agent files) |
| 6 | `DeepLoadExternalContextStage` | Ticket, docs, ADRs, conventions |
| 7 | `DeepInitialCommentStage` | Post the "review started" status |
| 8 | `DeepBusinessLogicValidationStage` | PR diff vs. Jira/Linear acceptance criteria |
| 9 | `CreateSandboxStage` | Acquire an isolated checkout |
| 10 | `FileFilterStage` | Drop binary/vendored files |
| 11 | `DeepCreatePrLevelCommentsStage` | Build the PR-level summary |
| 12 | `DeepValidateSuggestionsStage` | GitHub-only; 15 supported code extensions |
| 13 | `DeepCreateFileCommentsStage` | Build per-file inline comments |
| 14 | `DeepAggregateResultsStage` | Merge all finding sources |
| 15 | `DeepFinishCommentsStage` | Publish comments |
| 16 | `DeepRequestChangesOrApproveStage` | Submit `APPROVE` / `REQUEST_CHANGES` |

### `Orchestrator.ProcessReview`

The single call the worker makes (`orchestrator.go:189`) runs 8 numbered steps:

1. **Repository policy gate** — monitoring-paused / bot-excluded / branch-excluded → `ReviewStateSkipped`
2. **Diff filtering** — reassembles the diff so `ignoredPaths` never reach the LLM
3. **State → PROCESSING** (fails loudly if the row cannot be persisted)
4. **Parse diff** — `diff.ParseUnifiedDiff` with hard caps: 100k lines, 1000 files, 10k hunks
5. **Sandbox lease** — `Acquire(prKey)`; failure is non-fatal, review continues self-contained
6. **Deterministic rules** — `rulesEvaluator.EvaluatePatches`
7. **AST complexity + blast radius** — `ASTComplexityAnalyzer` → `priority.ScoreAndPrioritizeFiles`
8. **AI synthesis** — one `llmGateway.AnalyzeDiff` call

Then: artifact archival, semantic suppression, syntax verification, `BatchInsertFindings`, auto-ticketing, a **mid-flight supersede check** (`HasNewerReviewForPR`) that aborts before posting if a newer review exists, SCM publish, and **two signed attestations** (in-toto v1 + SLSA v1.0).

### Guardrails worth knowing

- **Diff budget.** `MaxDiffBytesForLLM = 250 KB`. Above that the diff is *re-prioritized*, not truncated — high blast-radius files are packed until the limit, then it stops.
- **Hunk boundary validation.** `DiffBoundaryValidator` clips findings to lines present in the diff, preventing SCM `422 Unprocessable Entity` on comment placement.
- **Adversarial arbiter.** `HeuristicSkepticalArbiter` runs 7 deterministic rules — it drops speculative findings ("nitpick", "might want to consider"), disputes severity-inflated findings, and forces `Blocking = true` when ≥2 agents agree.
- **Supersede check.** If a newer review exists for the same PR, the in-flight one completes without posting — no PR comment races.
- **Prompt-injection firewall.** `agentfirewall.Firewall.DetectPromptInjection` runs on `DiffContent`, `PullTitle`, and `CustomRules` before every model call.

---

## The Agent Harness

`internal/agentharness` is a **provider-agnostic agent loop**. One engine, many roles — `AgentSpec` is pure data, so swapping the system prompt, tools, and policies instantiates a new role without forking the loop.

```go
type AgentSpec struct {
    ID, SystemPrompt, AgentName string
    Tools      ToolRegistry
    Policies   []AgentPolicy
    MaxSteps   int
    ResultToolName string          // the "done" tool
}
```

`GoAgentRunner.Run` loop, per step: policy `PrepareStep` → model invocation → tool dispatch → record `RunStep` → policy `ShouldStop`. It exits on a result-tool call, a zero-tool-call turn, a policy veto, or `MaxSteps`.

### The four policies

| Policy | Enforces |
|---|---|
| `BudgetPolicy` | Step-budget bands (free/encourage/urgent). Injects a note **only on band transition** to preserve implicit prefix caching. |
| `CompletionGatePolicy` | Vetoes stopping unless the done-tool fired **and** no critical-tier coverage target is pending. |
| `ForceFinalizePolicy` | In the last 2 steps, hard-restricts `ActiveTools` to the result tool. |
| `CompressionPolicy` | Context-window compression, emits `before/after/savedTokens`. |

### The verifier fails open

`contracts.Verifier` implementations **must** default to `keep = true`. `RunVerificationPass` implements this twice: a `defer recover()` turns any verifier panic into `Keep: true`, and a non-nil error becomes `Keep: true`. A finding is dropped **only** on an explicit `keep = false`.

The prompt contract is explicit: *"DROP the finding ONLY if you can actively REFUTE it… KEEP the finding (this is the DEFAULT) whenever you cannot refute it."*

### Tools

`BuildCompleteToolRegistry` builds **18 tools** — `grep`, `readFile`, `listDir`, `findFile`, `checkTypes`, `getCallers`, `readReference`, `searchDocs`, `submitResult`, `submitVerdict`, `gitBlame`, `gitLog`, `gitDiffRange`, `runTests`, `symbolOutline`, `astQuery`, `importGraph`, `securityScanner`.

Thirteen of the eighteen are **read-only and cache-eligible**. `ToolCallCache` keys on `runID\x00toolName\x00stableInput` — the runID prefix prevents leakage across runs and tenants. Errors are **never** cached.

`BuildFinderToolRegistry` builds the lean 5-tool set (`readFile`, `listDir`, `grep`, `getCallers`, `gitDiff`) used by the review finder agent. `readFile` is **outline-first**: above 500 lines it returns a symbol outline instead of content.

---

## LLM Gateway & BYOK

`internal/llm/gateway.go` is a thin facade over `Engine`, which resolves a module from `kernel.DefaultRegistry` and calls `mod.Execute`.

### Providers

| Provider | Target API | Auth |
|---|---|---|
| `openai` | `api.openai.com/v1` | `Bearer` |
| `anthropic` | `api.anthropic.com/v1` | `x-api-key` |
| `google_gemini` | `generativelanguage.googleapis.com` | `x-goog-api-key` |
| `google_vertex` | `aiplatform.googleapis.com` | `Bearer` |
| `amazon_bedrock` | `bedrock-runtime.{region}.amazonaws.com` | `AWSBearerToken` |
| `open_router` | `openrouter.ai/api/v1` | `Bearer` |
| `azure` | `{resource}.openai.azure.com` | `api-key` |
| `novita` | `api.novita.ai/v3/openai` | `Bearer` |
| `moonshot` | `api.moonshot.ai/anthropic` | Anthropic-compatible |
| `zai` | `api.z.ai/api/anthropic` | Anthropic-compatible |

Plus Groq, Cohere, xAI, Mistral, DeepSeek, MiniMax, Alibaba, and vLLM wired through `GatewayOption`.

### Four independent failover layers

1. **Slot cascade** — stored BYOK → task override → managed config → env default → explicit `req.Model`
2. **Circuit breaker** — `ProviderBreakerRegistry`, 5 failures / 30 s gateway, 5 / 60 s engine
3. **Single-hop model fallback** — `FallbackSlot` has **no** `Fallback` field, so the compiler forbids recursive cascades
4. **Transient retry** — one retry with jittered backoff, only for `CategoryTransient`

`ClassifyLLMError` sorts failures into 11 categories. `AUTH_INVALID`, `QUOTA_EXCEEDED`, and `CONTENT_FILTER_BLOCKED` are terminal — never retried.

### BYOK (Bring Your Own Key)

Organizations supply their own provider credentials; those calls never consume platform quota.

- Keys are **AES-256-GCM encrypted** before storage (`SCANDRIX_ENCRYPTION_KEY`, SHA-256 → 32 bytes). If the key is unset outside development/test, the process **panics**.
- Stored per-workspace as JSON under the organization parameter `byok_config`.
- `BYOKConcurrencyLimiter` enforces four gates: max concurrency, RPM minimum interval, TPM reservoir, and cooldown on provider 429.
- BYOK **bypasses the monthly quota only** — the per-minute burst cap still applies as a runaway-loop guard.

### Token budgeting

`TokenBudgetLimiter` is an in-memory map keyed by workspace. Unknown workspaces auto-provision a Community baseline of 500k tokens/month, 50k tokens/min burst. Real usage is pushed in from PostgreSQL via `SyncUsageFromDB`.

---

## SCM Providers

`internal/platform` is a hexagonal ports-and-adapters rewrite. Two abstraction seams exist:

**`SCMAdapter`** (12 methods) — thin, used by the review pipeline. Each provider registers itself in `init()`.

```go
type SCMAdapter interface {
    Provider() models.SCMProvider
    FetchPullRequest(ctx, repo string, pullNumber int) (*PullRequestDetails, error)
    FetchDiff(ctx, repo string, pullNumber int) (string, error)
    PostInlineComments(ctx, repo string, pullNumber int, comments []InlineCommentSpec) error
    PostReviewSummary(ctx, repo, pullNumber, summary string, conclusion ReviewConclusion) error
    SetCommitStatus(ctx, repo, commitSHA, contextName string, state CommitStatusState, ...) error
    ListBranches / GetFileContent / ApprovePullRequest / MergePullRequest
    VerifyWebhookSignature(secret string, payload []byte, signatureHeader string) bool
    ParseWebhookEvent(eventType string, payload []byte) (*WebhookEventData, error)
}
```

**`ICodeManagementService`** (~85 methods) — fat, tenant-resolved, behind `CodeManagementService` which resolves the provider per call from organization context.

| Provider | Adapter | Service | Notable |
|---|---|---|---|
| GitHub | 475 LOC | 2703 LOC | GitHub App token rotation (RS256 JWT, 9-min expiry), ETag conditional-request cache, GraphQL thread resolution |
| GitLab | 444 LOC | 2607 LOC | Discussions API, award-emoji reactions, pipeline triggers, audit events |
| Bitbucket | delegating | 4173 LOC | Cloud **and** Data Center — `isDataCenter` credential flag picks the implementation |
| Azure DevOps | 408 LOC | 3790 LOC | Git REST 7.1, branch policies, work items, service hooks |
| Forgejo | 369 LOC | 4500 LOC | Gitea API v1, per-host TLS override for self-hosted |

`cmd/server` currently composes **GitHub, GitLab, and Bitbucket** into the router. Azure DevOps and Forgejo are fully implemented but not wired into the server's provider map.

### Rate limiting

`RateLimitCoordinator` applies per-`org:provider` gating: a token bucket (GitHub 5000/hr, GitLab 2000, Bitbucket 1000, Azure 1500, Forgejo 3000), a circuit breaker, and a concurrency semaphore — in that order. Backoff uses full jitter.

---

## Database

PostgreSQL 16 + pgvector. Access is **`pgx/v5` with no ORM** — a thin `Client` wrapping `pgxpool`, exposed through a hand-written `Repository` with parameterized SQL per domain.

### Schema

**48 migrations**, 54 base tables + 13 partitions.

| Domain | Tables |
|---|---|
| Tenancy | `workspaces`, `account_profiles`, `users`, `auth`, `revoked_access_tokens`, `sso_configs`, `scim_groups` |
| Reviews | `tracked_repositories`, `pull_request_reviews`, `code_findings`, `review_rules`, `review_attestations`, `tracked_issues`, `finding_feedback` |
| Code analysis | `code_ast_nodes`, `code_ast_edges`, `security_memory` (`vector(1536)` + HNSW), `drixy_embedding_vectors`, `drixy_rules` |
| Messaging | `outbox_events`, `inbox_records`, `warehouse_domain_events`, `notification_channels` |
| Sessions | `cli_auth_sessions`, `cli_devices`, `team_cli_key` |
| Billing | `organization_billing_seats`, `organization_licenses`, `billing_transactions`, `plan_configurations`, `workspace_spend_limits` |
| Analytics | `analytics_pull_request_events` (range-partitioned) + `materialized_dora_daily_rollups` |
| Sandbox | `sandbox_leases` |
| MCP | `mcp_connections`, `mcp_integrations`, `mcp_integration_oauth` (separate `mcp-manager` schema) |

### The two tenant primitives

Everything tenant-scoped goes through one of these:

```go
func (c *Client) ExecWithTenant(ctx, tenantID uuid.UUID, fn func(tx pgx.Tx) error) error
func (c *Client) ExecAsSystem(ctx, fn func(tx pgx.Tx) error) error
```

Both run inside a transaction and set GUCs **transaction-locally**. `ExecAsSystem` is the cross-tenant bypass used by the outbox relay, pre-auth token lookups, SCIM, and the DORA cron — every one of those queries carries the RLS system-worker flag.

### Row-Level Security

**47 tables have RLS enabled; 42 also `FORCE`** — meaning the table owner cannot bypass them.

```sql
CREATE POLICY tenant_isolation_workspaces ON workspaces
    FOR ALL USING (
        current_setting('app.is_system_worker', true) = 'true'
        OR id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
    );
```

Three tables deliberately have **no** RLS: `inbox_records` (global dedup), `plan_configurations` (pricing catalog), and the three `mcp-manager` tables.

### Least privilege

`migrations/ops/002_least_privilege_runtime_role.sql` creates `scandrix_runtime` — `NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE`. Migrations read a **separate** `MIGRATION_DATABASE_URL` because `CREATE EXTENSION` is superuser-only; the API and worker containers only ever receive the unprivileged DSN. The RLS tests fail against a superuser by design.

---

## Security Model

### Authentication

Hand-rolled **HS256 JWT** (no JWT library). `VerifyToken` pins `alg == "HS256"`, recomputes HMAC-SHA256, uses `subtle.ConstantTimeCompare`, and requires `exp`/`iat`/`nbf` plus a role in `{owner, admin, member, viewer}`.

Passwords use **Argon2id** by default (bcrypt supported). Password-reset tokens are bound to the *current password hash*, so they self-invalidate on use.

### Two login flows

- **RFC 8628 device flow** — `CLIDeviceSession`, 600 s TTL, 5 s poll interval, ambiguity-free user-code alphabet (no `0`/`O`/`1`/`I`). `ConsumeAndGetSession` is atomic single-use. The workspace is bound on the browser-approval leg — redemption without a tenant is refused.
- **RFC 8252 loopback** — browser redirects to `127.0.0.1` with a 10-minute TTL.

### Webhook verification

Every provider is **constant-time**, and every function returns `false` on an empty secret (fail closed):

| Provider | Mechanism |
|---|---|
| GitHub | requires `sha256=` prefix, `hmac.Equal` on hex-decoded HMAC-SHA256 |
| GitLab | `subtle.ConstantTimeCompare` on `X-Gitlab-Token` |
| Bitbucket / Forgejo | `hmac.Equal` on `X-Hub-Signature` / `X-Gitea-Signature` |
| Azure DevOps | strips `Bearer `/`Basic `, then constant-time compare |
| Azure (token) | AES-256-CBC decrypt, PKCS7 unpad, constant-time compare |

Replay protection derives a deterministic UUIDv5 from the delivery ID (`X-GitHub-Delivery`, `X-Gitlab-Event-UUID`, `X-Hook-UUID`, …).

### Other controls

| Control | Where |
|---|---|
| **SSRF guard** | `internal/netguard` — scheme allow-list, rejects userinfo, re-validates **every redirect hop** |
| **Air-gap egress** | `internal/platform/security/airgap.go` — `http.RoundTripper` allow-list of hostnames/CIDRs |
| **Path traversal** | `internal/pathguard` — traversal + **symlink escape** containment |
| **Encryption** | `internal/security/kms` — envelope encryption with `ReEncrypt` for key rotation; 4-tier decrypt fallback |
| **Provenance** | `internal/provenance/intoto` — Ed25519 over DSSE, per-workspace derived keys, in-toto + SLSA v1.0 |
| **Command allowlist** | `internal/agents/tools/readonly_command.go` — binary allow-list, not a block-list |
| **Air-gapped licensing** | Optional `HardwareFingerprint` binding; `SCANDRIX_LICENSE_KEY_RING` supports key rotation overlap |
| **RBAC** | Two-axis `role × action × resource` matrix, enforced server-side per route |

> The command validator's rationale is worth quoting: *"A blocklist is trivially defeated: `cat x; rm -rf /` … all start with an allowed word, and the command was passed to `sh -c`, so every shell metacharacter was live."* It uses an allow-list of binaries plus a read-only `git` subcommand allow-list, and tokenizes with **no** variable expansion, globbing, or command substitution.

---

## Dashboard

`scandrix-dashboard/` — Next.js 14 App Router, React 18, TypeScript strict, Tailwind 3.4.

```jsonc
"dependencies": {
  "next": "^14.2.35", "react": "^18.3.1", "zod": "^3.24.2",
  "framer-motion": "^11.18.2", "lucide-react": "^0.475.0",
  "clsx": "^2.1.1", "tailwind-merge": "^2.6.0",
  "@vercel/analytics": "^2.0.1", "@vercel/speed-insights": "^2.0.0"
}
```

**No component library, no state manager, no chart library, no auth SDK.** Everything is hand-built:

- **31 UI primitives** in `src/components/ui/` — Button, Card, DataTable, Combobox, CommandPalette, Modal, Sheet, Tabs, Toast, …
- **Charts are hand-written SVG** — `InteractiveBarChart`, `TokenTimeSeriesChart`, `TeamActivityHeatmap`
- **State** is `useState`/`useReducer` + React Context
- **Auth** is hand-rolled HS256 verification with WebCrypto

### The BFF proxy

The browser never sees a token. `src/app/api/proxy/api/[...path]/route.ts`:

1. `hasValidOrigin` on any non-GET → 403
2. Path segment allow-list `/^[a-zA-Z0-9_-]+$/`; upstream rebuilt as `/api/v1/…`
3. Reads the `scandrix_token` httpOnly cookie → verifies → 401 `{error, refreshRequired}` on failure
4. Sets `Authorization: Bearer` **upstream**
5. **SSE passthrough** for `/events` — streamed with `X-Accel-Buffering: no`, and *no* abort signal (a 30 s deadline made live PR streams reconnect forever)
6. 1 MiB body cap — enforced both by `content-length` and by a streaming counter
7. Content-Type pinned to `application/json` so an upstream `text/html` cannot render on the dashboard origin

Server components bypass the proxy and call `SCANDRIX_API_URL` directly with an explicit bearer header — and `api-fetch.ts` **deletes** any caller-supplied `Authorization` first, so a browser cannot smuggle one in.

### Sessions

`scandrix_token` (15 min) and `scandrix_refresh` (30 days), both `httpOnly`, `sameSite: "lax"`, `secure` in production. `verifySessionToken` **fails closed** if `JWT_SECRET` is unset. Cross-tab rotation is serialized with `navigator.locks.request("scandrix:session-refresh")`.

`hasValidOrigin` checks the unforgeable `sec-fetch-site` header first and fails closed when `NEXT_PUBLIC_APP_URL` is unset — the previous `req.url` fallback became "whatever origin the request claims" behind a forwarding proxy.

### Routes

77 page/layout files across three route groups. `/` is the Cockpit. `/pull-requests/[repositoryId]/[prNumber]` is the Review Studio. `/setup/*` is a 12-step onboarding wizard.

### Hardening

`next.config.mjs` sets `X-Frame-Options: DENY`, HSTS (2 years, preload), `nosniff`, `Referrer-Policy`, `Permissions-Policy`, and a CSP with `frame-ancestors 'none'` and `object-src 'none'`. `script-src` allows `'unsafe-eval'` **only in development**. The Next image optimizer is disabled (`unoptimized: true`) as a mitigation for **GHSA-2xp9-vwfh-vxw4**.

Fonts are self-hosted Geist via `next/font/local`.

---

## The CLI

```bash
scandrix review --staged              # review staged changes
scandrix review --branch origin/main  # review a branch
scandrix review --fast --fail-on HIGH # fast pass, gate on HIGH
scandrix review --fix                 # apply AST patches
scandrix tui                          # Bubble Tea cockpit (4 tabs)
```

### Commands

`review` · `auth` · `config` · `hook` · `decisions` · `pr` · `rules` · `skills` · `status` · `update` · `schema` · `chat` · `tui` · `scan` · `diff` · `fix` · `export` · `pentest` · `mcp` · `server` · `trace` · `history` · `dry-run` · `ci` · `telemetry` · `version`

Plus root aliases `login`, `logout`, `whoami`.

### Output formats

`-f terminal | json | markdown | sarif | agent | prompt`, plus JUnit XML, GitLab Code Quality, SonarQube, CSV, and HTML.

### Git hooks

`scandrix hook install` injects delimited blocks into **`pre-commit`** and **`pre-push`** — non-destructively, preserving any existing user script. `removeHookBlock` deletes only the delimited block and removes the file only if nothing else remains. `SCANDRIX_SKIP_HOOK=1` bypasses.

The hooks directory is resolved through `pathguard.ResolveUnder`, so a `gitdir:` pointer in a `.git` **file** (linked worktree / submodule) cannot redirect the installer to write an executable hook outside the repo parent.

### The TUI

Four tabs — `TabFindings`, `TabDiff`, `TabStats`, `TabChat` — on Bubble Tea + Lip Gloss. It runs a fast local dry-run rules-only review at startup and jumps straight to chat on a clean tree.

### The MCP server

`scandrix mcp` speaks MCP over stdio for IDE agents. The in-process gateway registers 8 tools (`scandrix_scan`, `scandrix_review`, `scandrix_apply_fix`, `scandrix_trace_recall`, `scandrix_analyze_snippet`, `scandrix_catalog_search`, `scandrix_validate_syntax`, `scandrix_lookup_osv_vulnerability`) behind four roles — `reviewer`, `auditor`, `mutator`, `admin`.

**The MCP HTTP endpoint is closed by default.** `IsMcpServerEnabled()` returns `false` unless `SCANDRIX_MCP_SERVER_ENABLED` is explicitly set, and `http_server.go` returns **503** when no authenticator is supplied rather than serving unauthenticated.

### Config

Three layers, later wins:

```
defaults → ~/.scandrix/config.json → <repo>/.scandrix/config.json → env vars
```

`~/.scandrix/credentials.json` is written with dir `0700` and file `0600`.

The repo-level file is `.scandrix.yml` — bot name, ignore patterns, max comments, auto-approve, severity threshold, custom rules dir (default `.drixy/rules`), notification channels, and PM auto-ticket config.

---

## Configuration

`internal/config` reads **121 environment variables**; `.env.example` documents **398**.

Load order: `.env.local` → `.env` → `../.env.local` → `../.env`.

### Groups

| Group | Variables |
|---|---|
| **Runtime** | `APP_ENV`, `PORT`, `WEBHOOKS_PORT`, `WORKER_ROLE`, `WORKER_HEALTH_PORT`, `WORKER_CONCURRENCY`, `WORKER_DRAIN_TIMEOUT_MS` |
| **Datastores** | `DATABASE_URL`, `MIGRATION_DATABASE_URL`, `REDIS_URL`, `RABBITMQ_URL` |
| **Secrets** | `JWT_SECRET`, `KMS_MASTER_KEY`, `SCANDRIX_ENCRYPTION_KEY`, `SCIM_BEARER_TOKEN` |
| **SCM** | `GITHUB_TOKEN`, `GITLAB_TOKEN`, `BITBUCKET_USERNAME/PASSWORD`, `AZURE_DEVOPS_PAT`, `FORGEJO_TOKEN`, + 5 webhook secrets |
| **LLM** | 20+ provider keys: `ANTHROPIC_`, `OPENAI_`, `GEMINI_`, `DEEPSEEK_`, `OPENROUTER_`, `MISTRAL_`, `XAI_`, `GROQ_`, `COHERE_`, `MOONSHOT_`, `NOVITA_`, `MINIMAX_`, `ALIBABA_`, `BEDROCK_`, `VERTEX_`, `VLLM_` |
| **Model routing** | `AI_MODEL_DEFAULT`, `_FALLBACK`, `_TRIAGE`, `_LOGIC`, `_SECURITY`, `_THREAT_MODEL`, `_ARBITER`, `_SYNTHESIZER` |
| **Email** | `SMTP_HOST/PORT/USERNAME/PASSWORD/FROM`, `RESEND_API_KEY`, `TURNSTILE_SECRET_KEY` |
| **Billing** | `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET` |
| **Sandbox** | `SANDBOX_PROVIDER`, `E2B_API_KEY`, `E2B_TEMPLATE_ID`, `E2B_PROXY_*` |
| **Air-gap** | `AIR_GAPPED`, `AIRGAP_ALLOWED_HOSTS` |
| **Licensing** | `SCANDRIX_LICENSE_PUBLIC_KEY`, `SCANDRIX_LICENSE_KEY`, `SCANDRIX_LICENSE_KEY_RING`, `SCANDRIX_HARDWARE_FINGERPRINT` |

`.env` is gitignored from the first commit. `.env.example` and `.env.production.template` are the committed templates, and CI runs an awk-based audit that inspects **value shape** — it extracts the password component of any `://user:pass@` connection string and rejects anything that is not empty, `${VAR}`, or a placeholder.

---

## Infrastructure & CI

### Terraform

Flat resources across 14 `.tf` files — no modules.

- **VPC** — 2 public + 2 private subnets, IGW, optional NAT ($33/mo, off by default, replaced by 5 VPC endpoints)
- **ECS Fargate on ARM64/Graviton** — 3 task definitions (api / webhooks / worker), all Secrets Manager backed
- **Worker on `FARGATE_SPOT`** — interrupt-tolerant, autoscaled 1→4 on CPU at 70%
- **ALB** (optional) — TLS13-1-2-2021-06, deletion protection **on by default**, path rules split `/webhooks*` to the webhook target
- **Cloudflare tunnel** (on by default) — plus a zone-level rate-limit ruleset: 100 req/60s on `/webhooks`, 20 req/60s on `/auth`
- **CloudFront**, **ElastiCache Redis7**, **Secrets Manager**, **CloudWatch** alarms

Image tags **reject `:latest`** — all three variables refuse it and require an explicit tag.

### GitHub Actions — 13 workflows

| Workflow | Does |
|---|---|
| `ci.yml` | pgvector service, migrations, `scandrix_runtime` provisioning, `gotestsum -race`, cross-package coverage merge, Codecov |
| `pr-quality-gate.yml` | Race suite + all 10 binaries built |
| `golangci-lint.yml` | Lint gate |
| `e2e-matrix.yml` | pgvector + RabbitMQ, integration suite, path-filtered |
| `docker-build-scan.yml` | Buildx + **Trivy**, `exit-code: 1`, CRITICAL-only |
| `docker-publish.yml` | 5 services → GHCR, `linux/amd64` + `linux/arm64` |
| `cli-release.yml` | 5 targets, SHA256 checksums, **SLSA Level 3** provenance |
| `deploy-production.yml` | OIDC → ECR → ECS; manual runs require typing `DEPLOY` |
| `deploy-staging.yml` | `staging-<sha>` + `staging-latest` |
| `deploy-ec2.yml` | rsync + `docker compose` + health gate |
| `secret-scan.yml` | Gitleaks + gitignore guard + `.env.example` audit |
| `security-audit.yml` | `govulncheck` + `gosec` SARIF + dashboard `pnpm audit` |
| `terraform-plan.yml` | Two-tier trust split — no `id-token` on fork PRs |

Third-party actions are pinned to immutable SHAs, and release tags are validated with a shell `case` before any interpolation.

---

## Local Development

```bash
make build          # all 11 binaries → ./bin/
make test           # unit + integration
make test-race      # with -race
make lint           # go vet
make env-check      # validate environment contract
make docker-up      # full stack
make install        # install scandrix to ~/.local/bin
```

`cmd/envcheck` validates the environment against the contract — it has a generator (`env_contract_gen.go`) that derives the contract from the source so the two cannot drift.

### Compose topologies

| File | Topology |
|---|---|
| `docker-compose.yml` | 7 services — postgres, rabbitmq, redis, migrate, api, webhooks, worker |
| `docker-compose.dev.yml` | 6 services — per-service Dockerfiles, live-reload |
| `docker-compose.prod.yml` | 5 services — external Postgres, health-gated |
| `docker-compose.cluster.yml` | 14 services — 2× API, 4× worker, nginx LB, LocalStack |

RabbitMQ and Redis bind to **loopback only** and require passwords. Services run with `cap_drop: ALL` and `no-new-privileges: true` as uid/gid 10001.

### Code layout

`internal/codeanalysis/languages/detector.go` detects **20 languages** — Go, TypeScript, JavaScript, Python, Java, Kotlin, Rust, C/C++, C#, Ruby, PHP, Swift, Scala, SQL, Dockerfile, Terraform, YAML, JSON, Markdown. Native `go/parser`/`go/ast` analyzers exist for Go, Python, TypeScript, Java, Rust, and C++, each with `FindEnclosingFunction(line)` and cyclomatic complexity.

---

## Testing

```bash
go test ./...                              # 710 test files
go test -race -covermode=atomic ./...
go test -bench=. -benchmem ./test/benchmark/...
```

| Suite | Location | Covers |
|---|---|---|
| Unit | Co-located `*_test.go` | 710 files across the tree |
| Integration | `test/integration/` | Broker chaos, webhook idempotency, **RLS tenant isolation**, worker-pool deadlock, webhook↔worker contract |
| CLI | `test/cli/` | Auth, config, review, rules |
| Matrix | `test/matrix/` | Provider matrix |
| MCP Manager | `test/mcp_manager/` | OAuth, crypto, handlers, middleware, providers, repos |
| Benchmark | `test/benchmark/` | Micro-benchmarks + stress |
| Evals | `evals/` | 11-domain offline harness |

### The eval harness

`evals/engine_gate.go` runs **12 sanity checks** across profiles `harness`, `local`, `ci`. It is **deterministic and offline** — pure scoring functions, no live LLM calls.

| Domain | Measures |
|---|---|
| `anchoring` | Whether a comment attaches to a changed hunk |
| `dedup` | Near-duplicate finding collapse |
| `drixyrules` | Custom rule detection |
| `format` | Output formatter conformance |
| `investigation` | Evidence sufficiency |
| `parser` | Diff/JSON parse robustness |
| `promotion` | Suggestion promotion thresholds |
| `prsummary` | Summary quality |
| `severity` | Severity assignment |
| `structuredoutputs` | JSON schema conformance |
| `tracecontext` | Architectural-decision recall |

### Agent skills

23 SKILL.md bundles in `skills/` covering review, OWASP (SQLi/XSS/SSRF/IDOR/Broken Auth), secret scanning, crypto standards, infra hardening, input sanitization, supply chain, centralized config, business-rule validation, AST complexity, and architectural-decision tracing.

---

## Project Scale

```
2,366 Go files        ·  ~467,000 lines of Go
710 test files        ·  49 internal packages
11 binaries           ·  48 database migrations
54 tables             ·  42 RLS-forced
398 env vars          ·  13 CI workflows
77 frontend routes    ·  31 UI primitives
23 agent skills       ·  11 eval domains
```

Largest packages:

| Package | Files | LOC |
|---|---|---|
| `internal/review` | 288 | 73,948 |
| `internal/platform` | 164 | 57,353 |
| `internal/cli` | 292 | 49,393 |
| `internal/api` | 115 | 33,527 |
| `internal/core` | 158 | 30,447 |
| `internal/llm` | 106 | 15,509 |

---

## License

ScanDrix Enterprise Proprietary Software License Agreement, Version 2.0 (Commercial & Enterprise Distribution). See [`LICENSE`](./LICENSE).

> **Note:** `cmd/cli/main.go` carries an AGPL-3.0 header, which conflicts with the proprietary `LICENSE` file. Treat `LICENSE` as authoritative and resolve the header.

---

<div align="center">

**ScanDrix** — Autonomous AI Code Review & Security Gate

[Security](https://scandrix.dev/security) · [Terms](https://scandrix.dev/terms) · [Privacy](https://scandrix.dev/privacy) · [Subprocessors](https://scandrix.dev/subprocessors)

</div>