# Glossary

Canonical terms. Terminology drift caused the roles/tiers contradictions fixed in `decisions/ADR-0004`, so this list is normative: use these words in code, docs, UI, and contracts.

## Tenancy & identity

| Term | Meaning | Not to be confused with |
|---|---|---|
| **Workspace** | The tenant boundary. Every scoped row carries `workspace_id`; RLS keys on it. | "Organization" (below) — they are the same boundary, but the codebase says `workspace` |
| **Organization** | Customer/legal entity that owns workspaces and the license. Display name for a workspace. | Workspace (the row) |
| **Team** | A group inside a workspace; owns members and CLI keys. | Workspace |
| **Member / User** | A person in a team. `account_profiles` = login identity, `users` = person record. | Session |
| **Role** | One of `OWNER`, `ADMIN`, `MEMBER`, `VIEWER` (canonical, `enterprise/rbac`). Enterprise titles (Security Admin, Engineering Lead, Developer) map onto these. | Permission (an action on a resource) |
| **Permission** | An action-on-resource pair (`read`/`update`/`manage` × resource) evaluated by the policy engine. | Role |
| **Repository assignment** | Per-user repository allowlist (`user_repository_assignments`) narrowing a member's reach. | Repository tracking |
| **Seat** | A licensed user slot. Enforced per workspace against license `MaxSeats`. | Member (a member without a seat exists) |
| **Team CLI key** | `scandrix_`-prefixed machine credential scoped to a team with capabilities (repo config, rules manage). | Access token (JWT) |

## Licensing & plans

| Term | Meaning |
|---|---|
| **License** | Ed25519-signed envelope proving entitlement (customer, tier, seats, repos, features, expiry). Offline-verified. |
| **Plan** | A row in `plan_configurations` (COMMUNITY, DEVELOPER, TEAM, SCALE, ENTERPRISE) with quotas, pricing, and feature bundle. |
| **Entitlement** | The single resolved answer "what may this workspace do" — from signed license, plan row, or community default. Produced by `license.Resolver`; never computed ad hoc. |
| **Tier** | The plan name carried on a license or plan row (same vocabulary as plan). |
| **Feature flag (license)** | A gated capability (`FEATURE_SSO_SAML`, `FEATURE_SCIM_PROVISIONING`, …) checked via `Entitlement.Allows`. |
| **Feature flag (rollout)** | A release-track/audience/percentage flag in `featuregate` (unrelated to licensing). |
| **Community plan** | The unlicensed default: BYOK + 5 seats + 3 repos, no gated features. |

## Review domain

| Term | Meaning | Not |
|---|---|---|
| **Review** | One analysis run over a PR head SHA (`pull_request_reviews`). | Repository |
| **Finding** | A single issue produced by an agent/rule (`code_findings`) with category, severity, confidence. | Issue |
| **Issue** | A tracked work item, optionally auto-created to a PM tool (`tracked_issues`). | Finding |
| **Suggestion** | A finding with a concrete patch proposal. | Comment |
| **Verified** | The patch compiled and passed tests in the sandbox. Anything else posts as `unverified (<reason>)` — never silently as verified. | High confidence |
| **Confidence** | LLM self-score (0–1) used by the arbiter's 0.92 operating threshold. Not a calibrated probability. | Verification |
| **Rule** | A reusable check (built-in catalog or Drixy rule) evaluated against the diff. | Finding |
| **Attestation** | Signed record that a review ran under a specific configuration (`review_attestations`). | Audit log |
| **Dry run** | Review without posting comments. | Preview |

## Reliability & infrastructure

| Term | Meaning |
|---|---|
| **Outbox** | Row written in the same transaction as a state change, published later by the relay. Guarantees no lost event. |
| **Inbox** | Per-consumer dedup record making redelivery a no-op. |
| **DLQ** | Dead-letter queue: messages that exhausted 5 retries, parked for operators. |
| **Relay** | Worker loop that moves outbox rows onto RabbitMQ with per-tier priority. |
| **Claim** | Time-boxed, exclusive lease on an outbox/inbox row (`claimed_by`, `claim_expires_at`). |
| **Idempotency key** | `repo:pr:sha` (webhooks) or message id (consumers) that makes repeats harmless. |
| **RLS** | PostgreSQL row-level security keyed on `app.current_tenant_id`. The tenant enforcement point. |
| **Air-gapped** | Deployment with no route to the internet. Binary enforcement is Phase 1; until then the network is the control. |
| **SaaS / self-hosted** | ScanDrix-operated vs customer-operated deployment. |

## Status vocabulary (used across all docs)

| Label | Meaning |
|---|---|
| **BUILT / IMPLEMENTED** | Ships today; pointer to code included. |
| **PARTIAL** | Wired but incomplete (e.g. SCIM quota code unwired). |
| **STUB** | Endpoint/UI exists, returns placeholder. Not sellable. |
| **SPECCED** | Contracted, not built; tracked in `enterprise/IMPLEMENTATION.md`. |
| **TARGET** | Desired number, gated by measurement (`evaluation/methodology.md`). |
| **ADR** | Recorded decision (`decisions/`). Re-litigating one requires a new ADR, not an edit. |
