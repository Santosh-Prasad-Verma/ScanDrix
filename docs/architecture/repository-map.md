# Repository & Module Map

51 packages under `internal/`, 2 under `pkg/`, 13 under `cmd/`. Purpose line + anchor file each, so you extend the right package instead of creating a neighbour (see `decisions/ADR-0005-no-parallel-trees.md`).

## Binary matrix

| `cmd/` | Built into images | Composition root | Notes |
|---|---|---|---|
| `api` | ✅ | API + webhooks controllers, no crons | Only binary that loads the license manager today |
| `server` | ✅ (ENTRYPOINT) | API + sandbox pool + 12 crons | Monolith; full license & SCIM wiring |
| `webhooks` | ✅ | Ingestion + billing webhooks | Fire-and-forget by design |
| `worker` | ✅ | Consumers + relay + crons | `WORKER_ROLE=all\|codereview\|analytics` |
| `migrate` | ✅ | schema runner | Transactional, ordered |
| `cli`, `scandrix` | ✅ (cli) | Cobra tree | `scandrix` is an alias entrypoint |
| `mcp-manager` | ✅ | MCP registry | separate schema |
| `ast-cli`, `analytics-cli` | ✅ | local tools | no server role |
| `try` | ✅ | playground | public CORS `*` |
| `devserver`, `envcheck` | ❌ | dev/ops | not in `make build` |
| `scandrix-keygen` | ✅ | license authority CLI | Keypair generation, signing, and verification |

## `internal/` map

| Package | Purpose | Anchor |
|---|---|---|
| `api` | chi router, ~50 controllers, middleware, SSE | `router.go` |
| `api/controllers` | HTTP handlers grouped by domain | `license_controller.go`, `sso_config_controller.go` |
| `api/middleware` | feature gate (entitlement), tier rate limit, security headers, CORS | `feature_gate.go` |
| `api/dtos` | request/response payloads | `governance_dto.go` |
| `auth` | HS256 JWT, OAuth, SAML/OIDC SSO, CLI device/loopback, helpdesk tokens, mailer | `auth.go`, `sso.go` |
| `auth/{sso,oauth,cliauth,clitokens,mailer}` | SSO protocol engine, providers, device flow, team keys, SMTP | `sso/` |
| `enterprise/license` | Ed25519 licensing, entitlement resolution, plan quotas, model gating | `validator.go`, `entitlement.go`, `resolver.go` |
| `enterprise/rbac` | canonical role matrix, chi policy middleware, per-repo assignments | `policy.go`, `middleware.go` |
| `enterprise/audit` | audit envelope, async listener, SIEM (CEF/syslog) export, 13 handlers | `events.go`, `siem_exporter.go` |
| `enterprise/scim` | RFC 7644 SCIM server, deprovisioning | `handler.go` |
| `enterprise/classification` | conventional-commit PR classifier for DORA | `pr_classifier.go` |
| `billing` | provider-agnostic types, dunning, idempotency, webhooks | `types.go` |
| `billing/razorpay` | Razorpay client/service/verifier | `service.go` |
| `database` | pgx pool + repository façade over ~20 repos | `repository.go` |
| `queue` | AMQP broker (quorum/DLX/delayed), consumer pool, resilience | `rabbitmq.go`, `resilience.go` |
| `queue/relay` | outbox store/publisher, inbox dedup, dispatcher | `outbox.go`, `inbox.go` |
| `queue/consumer` | review task payload, consumer, worker pool | `models.go` |
| `review` | orchestrator, diff handling, stream hub (SSE) | `orchestrator.go` |
| `review/aiengine` | context assembly, LLM response processing, reference detector | `context_pack_assembler.go` |
| `review/pipeline` | staged review pipeline | `stages/` |
| `rules` | rule catalog + evaluator | `evaluator` |
| `rules/drixy` | Drixy rules engine, rule library, likes, embeddings | `infrastructure/adapters/services/` |
| `llm` | gateway, routing, budget limiter, embeddings | `gateway` |
| `llm/byok` | per-workspace encrypted keys, managed slot resolution | `byok/` |
| `codeanalysis` | AST analysis | `ast/` |
| `codeowners`, `security` | CODEOWNERS logic, security helpers | — |
| `organization` | workspace/team/member/BYOK use-cases, org parameters | `module.go` |
| `identity` | signup/auth/profile use-cases (**secondary RBAC — converging, ADR-0004**) | `application/auth_usecases.go` |
| `analytics` | pricing catalog, token usage, spend limits | `pricing/`, `usage/`, `spendlimit/` |
| `cockpit` | DORA/quality metrics, tier policy for self-hosted | `domain/tier_policy.go` |
| `cron` | 15 scheduled jobs (watchdog, seat pruner, DORA, sandbox reaper…) | `scheduler.go` |
| `featuregate` | release-track/audience/rollout flags, snapshot, cloud decision path | `engine.go`, `service.go` |
| `integrations` | notifiers: Jira, Linear, Slack, Discord, Teams, Zoho | — |
| `issues` | issue tracking + auto-ticket manager (`integrations/pm`) | — |
| `platform` | SCM adapters: github, gitlab, bitbucket, azure, forgejo + factory | `factory/` |
| `scm` | diff fetch router | — |
| `platformdata` | PR warehouse repositories/use-cases | — |
| `centralizedconfig` | config tree, PR service, storage adapters | `infrastructure/` |
| `sandbox` | sandbox providers, lease manager, reaper | `provider.go`, `lease/` |
| `mcp`, `mcp/manager` | MCP server + manager service | — |
| `agent*` (`agentharness`, `agents`) | agent run contracts, business-rules + conversation agents | `agentharness/runner` |
| `telemetry` | metrics, tracing, product analytics, self-hosted beacon | `beacon/` |
| `storage` | artifact client (S3/Appwrite) | — |
| `provenance`, `blueprint`, `finetuning`, `diagnostics`, `usecases`, `shared`, `common`, `config`, `core`, `clireview`, `ci`, `try`, `worker`, `notifications`, `scandrix` | supporting domains: run contracts, prompts/email templates, env loading, domain entities+migrations mirror, CLI review engine, CI integration, playground, worker role/health, notification channels, misc | `common/`, `config/config.go` |

## `pkg/` (shared, importable from anywhere)

| Package | Purpose | Anchor |
|---|---|---|
| `models` | cross-domain DB/JSON structs: `AccountProfile`, `UserRole`, `OrganizationLicense`, `AuditLogRecord`, billing records | `models.go` |
| `crypto` | AES-256-GCM + key rotator | `crypto.go`, `rotator.go` |

## Where to put new code (decision rule)

1. Feature belongs to an existing domain → extend that package.
2. Feature needs its own table + RLS policy → new migration in the owning domain's area, not a new schema.
3. Feature is cross-cutting infrastructure (queue, crypto, telemetry) → `pkg/` if import-agnostic, else `internal/common`.
4. New top-level `internal/` package needs a one-paragraph ADR citing why the neighbour couldn't carry it (ADR-0005).
