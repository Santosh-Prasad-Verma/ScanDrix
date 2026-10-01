# ScanDrix Enterprise Edition — Product Requirements Document (PRD)
**Version:** 2.0 Enterprise  
**Status:** Approved & Target Architecture  
**Author:** ScanDrix Architecture & Product Engineering  
**Confidentiality:** Proprietary & Confidential — ScanDrix  

---

## 1. Executive Summary & Vision

### 1.1 Product Vision
**ScanDrix Enterprise** is an autonomous AI code review, AppSec gate, and engineering intelligence platform built natively in **compiled Go**.

Its design bets, stated as architecture (not as claims about competitors): multi-agent review with an adjudication threshold instead of single-pass commenting; sandbox execution of proposed patches before posting; offline Ed25519 license verification; and a deployment ladder (SaaS → dedicated → VPC → self-hosted → air-gapped) where each rung reduces external dependencies.

### 1.2 Core Value Proposition
1. **Sandbox-checked code suggestions:** Proposed patches are compiled, linted, and test-checked in an isolated sandbox before posting. Suggestions that cannot be verified are **labeled unverified, never silently posted as verified** (fallback policy: REQ-2.3). "Zero hallucinations" is not promised — verification coverage is the metric (§6).
2. **Adversarial Multi-Agent Deliberation:** Three specialist agents (Software Architect, AppSec Red-Teamer, Performance Engineer) analyze the diff in parallel under an Arbiter Judge with a 0.92 operating threshold (threshold calibration: REQ-1.2). Only findings surviving adjudication and (where buildable) sandbox verification are posted.
3. **Sovereign & Air-Gapped Deployments (BYOK):** On-premise execution with self-hosted LLMs (DeepSeek-R1, vLLM, Ollama) where REQ-4.1 holds: external egress is limited to an explicitly allowlisted set, verified by a quarterly egress audit. Telemetry off-switches exist today (beacon honors `SCANDRIX_TELEMETRY_DISABLED`; Sentry is inert without `SENTRY_DSN`; PostHog is cloud-mode only), and the `AIR_GAPPED` runtime enforcement gate is **IMPLEMENTED** (`internal/platform/security/airgap.go`).
4. **Compiled High-Performance Engine:** Go 1.25 services with AST-assisted review and horizontal RabbitMQ scaling. Resource claims are the measured NFRs in §5.1, not adjectives.

---

## 2. Target Personas & Stakeholders

| Persona | Role | Primary Pain Points Solved |
| :--- | :--- | :--- |
| **Alex — VP of Engineering** | Engineering Leadership | Slow PR review turnaround, high reviewer fatigue, lack of objective DORA metrics, unpredictable release velocity. |
| **Samantha — CISO / Head of AppSec** | Security & Compliance | Leaked secrets in git, dependency vulnerabilities, non-compliant PRs merging without review, strict data-residency laws preventing cloud LLM usage. |
| **Marcus — Staff Software Architect** | Technical Standards | Inconsistent code style, architectural drift, leaky abstractions, junior devs repeatedly committing anti-patterns. |
| **Elena — Senior Software Engineer** | Core Contributor | Annoyed by noisy AI bots commenting on formatting; wants instant, high-impact suggestions that compile and can be merged with 1 click. |

---

## 3. Strategic Differentiators (design positions, not competitor measurements)

> The quadrant chart with numeric competitor scores previously shown here is withdrawn: the coordinates
> were illustrative, not measured, and do not belong in a production spec. Positioning below describes
> ScanDrix architecture only. No claim is made about any competitor's internals.

1. **Sandboxed Verification:** Suggestions execute in isolated containers/microVMs against the project's own build and tests before posting; unverifiable ones are labeled, not posted as verified.
2. **Multi-Agent Deliberation:** Parallel specialist agents plus an Arbiter Judge with a calibrated operating threshold, instead of a single scoring pass.
3. **Hardware-Signed Offline Licensing:** Ed25519 verification with no outbound calls on the verify path.
4. **Compiled Go services:** Single static binaries, multi-core workers, no interpreter runtime in production images. Memory/latency claims are §5.1 measurements, not multiples of any other stack.

---

## 4. Epic & Feature Requirements

### Epic 1: Autonomous Multi-Agent Deliberation Council
* **REQ-1.1:** The engine must spawn three parallel specialized analysis agents for every eligible pull request diff:
  * *Lead Architect Agent:* Focuses on OOP/SOLID/FP design, modularity, idiomatic conventions, and readability.
  * *AppSec Red-Team Agent:* Analyzes CWE/OWASP vulnerabilities, tainted user inputs, improper authorization, encryption flaws, and secret exposure.
  * *Performance & Reliability Agent:* Identifies memory leaks, unclosed streams/goroutines, N+1 queries, race conditions, and quadratic time complexity.
* **REQ-1.2:** Findings from all three agents must be piped to the **Arbiter Judge Agent**, which performs deduplication, weighs severity, discards subjective nits, and applies the **0.92 operating threshold** before accepting an issue. The threshold is an LLM self-score operating point, **not a calibrated probability**: it ships as a tunable, and its value is frozen only after measurement on a golden PR corpus (`evals/` anchoring/dedup/severity/format suites extended with precision/recall ground truth). No customer-facing accuracy claim may cite 0.92 as a guarantee.
* **REQ-1.3:** The final review comment must render a structured, clean executive summary table, highlighting categorized issues with rationale and verifiable patches.

### Epic 2: Sandboxed Suggestion Execution & Verification (MicroVM)
* **REQ-2.1:** Every code suggestion generated by ScanDrix must be validated through an isolated execution environment (Docker container, Firecracker microVM, or E2B sandbox).
* **REQ-2.2:** The sandbox must apply the proposed `git diff` patch on the base branch commit and execute the project’s native linter and build command (e.g., `go test -c`, `cargo check`, `npm run build`, `pytest`).
* **REQ-2.3:** If the sandbox reports a compilation or test failure, the error log must be fed back to the LLM for self-correction (max 2 repair iterations). If it still fails, the suggestion is suppressed. Degraded-mode policy (explicit, no silent behavior):
  * Sandbox infrastructure down → review proceeds with static/LLM findings labeled `unverified (sandbox unavailable)`; nothing is posted as `verified`.
  * Non-buildable change (docs-only, config-only, unsupported language, missing toolchain) → sandbox step is skipped by rule and the finding is labeled `unverified (not buildable: <reason>)`.
  * Suppression is per-suggestion; a fully suppressed review still posts its summary with the verification coverage attached. A review that posts zero findings because everything was suppressed must say so.

### Epic 3: Semantic AST Call-Graph Impact Analysis
* **REQ-3.1:** Tree-sitter parsing in Go supporting the primary languages (Go, TypeScript, JavaScript, Python, Rust, Java, C#, C++, Ruby, PHP, Kotlin, Swift), delivered per-language behind a parse-success gate (≥99% of files <500KB parse without error on the golden corpus). **Build constraint (blocking decision for Phase 2):** Tree-sitter bindings require CGO, but the production `Dockerfile` builds with `CGO_ENABLED=0` and `go.mod` carries no tree-sitter dependency today. Phase 2 must either enable CGO in the builder image (with a multi-arch `amd64/arm64` plan) or select pure-Go parsers per language — the epic is not achievable under the current build flags as-is.
* **REQ-3.2:** When a function, method, or class signature is modified, the engine must extract the symbol name and scan the repository index to identify all call sites across unchanged files.
* **REQ-3.3:** The PR review must explicitly alert engineers to breaking downstream changes (e.g., "Function `CalculateDiscount` modified: 4 call sites in `billing/` may experience runtime errors").

### Epic 4: Sovereign Air-Gapped Operation & BYOK
* **REQ-4.1:** The system must function disconnected from external internet access when configured in air-gapped mode. Honest status: per-service off-switches exist and are verified — beacon honors `SCANDRIX_TELEMETRY_DISABLED` (`internal/telemetry/beacon/transport.go`), Sentry is inert without `SENTRY_DSN` (`core/infrastructure/config/sentry.go`), PostHog is consulted only on the cloud path (`internal/featuregate/service.go`), and `is_air_gapped` is stored per license. The single `AIR_GAPPED` runtime enforcement gate is **IMPLEMENTED** (`AirGapGate` in `internal/platform/security/airgap.go` wrapping `http.RoundTripper` to deny non-local egress, honoring loopback, private CIDRs, and `AIRGAP_ALLOWED_HOSTS`).
* **REQ-4.2:** Native support for local inference endpoints using standard OpenAI/Anthropic wire protocols (vLLM, Ollama, TGI, LocalAI, DeepSeek-R1, Llama 3.3).
* **REQ-4.3:** Bring-Your-Own-Key (BYOK) encryption: Enterprise customer API keys are encrypted at rest using AES-256-GCM with tenant-isolated KMS keyrings. Keys are decrypted strictly in volatile RAM during active inference calls.

### Epic 5: Enterprise Identity & Directory Provisioning (SAML & SCIM)
* **REQ-5.1:** SAML 2.0 Single Sign-On supporting Okta, Microsoft Entra ID (Azure AD), Google Workspace, PingFederate, and Keycloak with cryptographic X.509 signature verification. OIDC (Google Workspace hosted domains, Entra ID) is supported by the protocol engine and shares the same session and provisioning path.
* **REQ-5.2:** SCIM 2.0 Server (`/scim/v2/Users`, `/scim/v2/Groups`) to automate real-time user provisioning, role synchronization, and deprovisioning when employees join or leave the organization. Provisioning must enforce seat quota (409 Conflict on exhaustion) and persist to PostgreSQL (no in-memory state).
* **REQ-5.3:** Role-Based Access Control (RBAC) with four canonical roles enforced in code (`internal/enterprise/rbac/policy.go`): `OWNER`, `ADMIN`, `MEMBER`, `VIEWER`. Enterprise display titles map onto canonical roles and must not introduce parallel matrices:

  | Enterprise title | Canonical role | Notes |
  | :--- | :--- | :--- |
  | Owner | `OWNER` | Full authority (`manage all`) |
  | Security Admin | `ADMIN` | Manage rules/reviews/repos/integrations/members; read audit + billing |
  | Engineering Lead | `MEMBER` + repo allowlist | `user_repository_assignments` scope for lead-level isolation |
  | Developer | `MEMBER` | Create/read reviews; read rules/repos |
  | Auditor (read-only) | `VIEWER` | Reviews/rules/repos read; no mutations |

  No new role system may be added; `internal/identity` and `internal/auth.RoleGuard` converge onto this matrix (see Epic 8, REQ-8.8).
* **REQ-5.4:** MFA (TOTP, with WebAuthn as follow-up) for password and SSO-fallback logins; enforceable per workspace (`enforce_mfa`). Repeated failures trigger lockout/backoff.

### Epic 6: Engineering Intelligence & DORA Metrics Warehouse
* **REQ-6.1:** Real-time ingestion of PR lifecycle events into a high-performance PostgreSQL warehouse with time-partitioned tables.
* **REQ-6.2:** Automated computation of standard DORA metrics:
  * *Deployment Frequency (DF)*
  * *Lead Time for Changes (LTC)*
  * *Change Failure Rate (CFR)*
  * *Mean Time to Restore (MTTR)*
* **REQ-6.3:** Proprietary ScanDrix quality metrics:
  * *Review Turnaround Velocity (P50/P90)*
  * *Code Suggestion Acceptance Rate*
  * *Vulnerability Elimination Index (Pre-merge vs Post-merge)*
  * *Hotspot Regressions (files modified repeatedly due to bugs)*

### Epic 7: Asymmetric Ed25519 Cryptographic Licensing
* **REQ-7.1:** License entitlements must be validated completely offline using asymmetric Ed25519 digital signatures.
* **REQ-7.2:** Signed license payloads must enforce:
  * `CustomerName` and `CustomerID`
  * `Tier` (Community, Developer, Team, **Scale**, Enterprise — five tiers, matching `plan_configurations` seeds)
  * `ExpiresAt` (Timestamp)
  * `MaxSeats` (Developer seat cap)
  * `MaxRepositories` (Linked repo cap)
  * `Features` (flags below; PRD short names map 1:1 to code `FeatureFlag` values — no new names may be introduced):

  | PRD short name | Code `FeatureFlag` |
  | :--- | :--- |
  | `BYOK` | `FEATURE_BYOK_ENCRYPTION` |
  | `AIR_GAPPED` | `FEATURE_AIR_GAPPED` |
  | `MULTI_AGENT` | `FEATURE_MULTI_AGENT_DELIBERATION` |
  | `SCIM` (+ SSO) | `FEATURE_SCIM_PROVISIONING`, `FEATURE_SSO_SAML` |
  | `DORA_METRICS` | `FEATURE_DORA_METRICS` |
  | (custom rules) | `FEATURE_CUSTOM_RULES` |
  | (audit warehouse) | `FEATURE_AUDIT_WAREHOUSE` |

  * `HardwareFingerprint` (optional server/cluster UUID lock; enforced only when the deployment declares an expected fingerprint, otherwise ignored for backward compatibility)
  * `KeyID` (optional signing-key identifier enabling zero-downtime rotation; absent = primary key)
* **REQ-7.3:** Sub-millisecond license verification with a **7-day grace period** (`LicenseGracePeriod = 7*24h` in code) with warnings before hard gating.
* **REQ-7.4 (rotation & revocation):** Key rotation via overlapping `KeyID` ring (new licenses mint under the new key while old ones still verify); revocation via expiring/re-issuing with a revocation list checked at boot and by the daily seat-pruner cron. Compile-time-only single public key is not acceptable as the steady state.

### Epic 8: Security Operations & Compliance Hardening
* **REQ-8.1 (audit integrity):** Hash-chained tamper-evident audit log (implemented) plus either RFC 3161 trusted timestamps or a documented downgrade of the SOC 2 claim — the claim and the mechanism must match.
* **REQ-8.2 (retention):** Configurable data-retention purge (code snippets, audit rows, sandbox artifacts) enforced by a scheduled job, plus the zero-retention mode (metrics + hashes only). A retention field without a purger is not compliant.
* **REQ-8.3 (secret rotation):** Documented rotation procedures with overlap for JWT secrets, `SCIM_BEARER_TOKEN`, webhook secrets, KMS keys, and the license authority keypair. Every secret has exactly one source (env/secret manager) and one rotation runbook.
* **REQ-8.4 (SCIM hardening):** Rate limits on `/scim/v2/*`, bearer-token rotation without downtime, RFC 7644 pagination/filter conformance targets.
* **REQ-8.5 (backup/DR):** RPO/RTO targets with tested restore for PostgreSQL (licenses, seats, audit), Redis (locks/quotas), and RabbitMQ (durable quorum queues). Seat/quota state must survive failover without double-counting.
* **REQ-8.6 (supply chain):** Pinned toolchains, lockfiles, dependency audit in CI, signed release artifacts, and a license-compliance gate (no AGPL statically linked into commercial binaries) with the tool named per gate.
* **REQ-8.7 (air-gap operations):** Offline update bundle flow (signed images + migration pack + vuln-feed mirror refresh) — air-gap is a lifecycle, not just an egress rule.
* **REQ-8.8 (RBAC convergence):** A single canonical RBAC matrix (REQ-5.3). Adapter shims where migration is staged, but no second permission table.

---

## 5. Non-Functional Requirements (NFRs)

### 5.1 Performance & Latency (each with observation method)
* **PR Review Throughput:** P50 < 20 seconds, P95 < 45 seconds for pull requests up to 50 files and 2,500 lines of diff, measured by worker span metrics (`review.duration_seconds` histogram) over a 7-day rolling window. Latency budget (must sum under 45s at P95): agent fan-out slice + arbiter slice + **sandbox slice ≤ 20s per PR total** (enforced by capping verified suggestions per PR; overflow findings post as `unverified (budget exhausted)`). A single suggestion may consume up to 3×15s sandbox attempts; the PR-level cap is what keeps the P95 honest. Any phase exceeding its slice degrades the label, never the silence.
* **Memory Efficiency:** Worker P95 RSS < 350 MB under 8 concurrent reviews; idle < 150 MB. Measured via container metrics in the scale sizing run (§4 of TRD), not developer laptops. The prior 75 MB idle target is withdrawn as unachievable with the pgx + gateway baseline.
* **Database Query Performance:** P99 < 120ms for dashboard queries over the partitioned warehouse, measured against 1M+ seeded review runs in staging, served from `materialized_dora_daily_rollups` (TRD §3.5) — raw-event scans are not on the dashboard path.

### 5.2 Reliability & Fault Tolerance
* **System Uptime:** 99.9% single-node, 99.95% clustered (multi-AZ PG + mirrored quorum queues). 99.99% requires the Global Monorepo tier plus tested AZ-failover runbook; do not quote it below that tier.
* **Message Delivery Guarantee:** At-least-once delivery with bounded loss windows: durable RabbitMQ quorum queues + persistent messages + consumer acks + DLQ after 5 backoff retries (all IMPLEMENTED: `internal/queue/rabbitmq.go` durability/quorum/persistence, inbox dedup). "Zero message loss" is not claimed — broker loss windows (e.g., unacked in-flight during hard failover, operator purge) are covered by the reconciliation sweep and DLQ alerting, with RPO measured by the queue-drain runbook, not asserted.
* **Idempotency:** Webhook processing must use distributed Redis/PostgreSQL locks (`x-claim-key`) ensuring duplicate Git provider webhooks never execute twice, covered by the inbox-dedup integration test.

### 5.3 Security & Compliance
* **SOC 2 Type II & ISO 27001 Alignment:** Immutable audit logs capturing all administrative actions, policy modifications, and SSO logins; trusted-timestamp mechanism per REQ-8.1 (RFC 3161 or documented equivalent — claim must match implementation).
* **Zero Data Retention Option:** Per REQ-8.2 — flag plus enforcing purger; only aggregated metrics and cryptographic hashes are stored.
* **Least Privilege Isolation:** Native PostgreSQL Row-Level Security (RLS) enforcing tenant isolation at the database kernel level (`app.current_tenant_id`), verified by the RLS test matrix (no `X-Tenant-ID` header trust).

---

## 6. Success Metrics & KPIs

```
┌──────────────────────────────────────────────────────────────────────┐
│              Enterprise Success Scorecard (TARGETS)                  │
│  Every quality number below is a target gated by measurement.        │
│  It may not appear in customer material until the eval gate passes:  │
│  frozen golden PR corpus with ground-truth labels, precision/recall  │
│  computed in CI (`evals/`), threshold + corpus version recorded      │
│  alongside the number.                                               │
├──────────────────────────────────┬───────────────────────────────────┤
│ Metric                           │ Enterprise Target                 │
├──────────────────────────────────┼───────────────────────────────────┤
│ PR Suggestion Acceptance Rate    │ > 68% merged without edit         │
│ False Positive Rate              │ < 4% reported false alarm         │
│ Review Latency (P90)             │ < 45 seconds                      │
│ Security Vulnerability Catch     │ > 94% CWE pre-merge               │
│ Air-Gapped Egress                │ 0 findings in quarterly egress audit (allowlisted VPC endpoints exempted and listed) │
│ License Check Overhead           │ < 1ms P99 per entitlement resolution (measured, cached) │
└──────────────────────────────────┴───────────────────────────────────┘
```
