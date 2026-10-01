# ScanDrix Enterprise Edition — Product Requirements Document (PRD)
**Version:** 2.0 Enterprise  
**Status:** Approved & Target Architecture  
**Author:** ScanDrix Architecture & Product Engineering  
**Confidentiality:** Proprietary & Confidential — ScanDrix  

---

## 1. Executive Summary & Vision

### 1.1 Product Vision
**ScanDrix Enterprise** is the next-generation autonomous AI code review, AppSec gate, and engineering intelligence platform built natively in **compiled Go**. 

While first-generation AI review bots (such as Kodus AI, SonarQube, and legacy script-based bots) rely on naive single-pass LLM prompts, single-threaded Node.js runtime engines, and untested hallucinated code suggestions, ScanDrix Enterprise delivers an **uncompromising, verifiable, multi-agent platform designed for mission-critical engineering organizations, defense, financial institutions, and global tech enterprises**.

### 1.2 Core Value Proposition
1. **Zero Hallucinated Code Suggestions:** Every single code suggestion is compiled, linted, and verified in an isolated execution sandbox before being presented to developers.
2. **Adversarial Multi-Agent Deliberation:** Instead of a single LLM prompt making subjective comments, a 3-agent specialist council (Software Architect, AppSec Red-Teamer, Performance Engineer) debates the diff under the adjudication of a neutral Arbiter Judge. Only high-confidence, non-trivial findings survive.
3. **100% Sovereign & Air-Gapped Deployments (BYOK):** Full on-premise execution with self-hosted LLMs (DeepSeek-R1, vLLM, Ollama) and zero telemetry data egress.
4. **Compiled High-Performance Engine:** Built in Go 1.25 with sub-second AST analysis, low memory footprint, and horizontal RabbitMQ scaling capable of handling monorepos with 10,000+ engineers.

---

## 2. Target Personas & Stakeholders

| Persona | Role | Primary Pain Points Solved |
| :--- | :--- | :--- |
| **Alex — VP of Engineering** | Engineering Leadership | Slow PR review turnaround, high reviewer fatigue, lack of objective DORA metrics, unpredictable release velocity. |
| **Samantha — CISO / Head of AppSec** | Security & Compliance | Leaked secrets in git, dependency vulnerabilities, non-compliant PRs merging without review, strict data-residency laws preventing cloud LLM usage. |
| **Marcus — Staff Software Architect** | Technical Standards | Inconsistent code style, architectural drift, leaky abstractions, junior devs repeatedly committing anti-patterns. |
| **Elena — Senior Software Engineer** | Core Contributor | Annoyed by noisy AI bots commenting on formatting; wants instant, high-impact suggestions that compile and can be merged with 1 click. |

---

## 3. Strategic Differentiators vs. Legacy Competitors

```mermaid
quadrantChart
    title Enterprise Code Review Platform Landscape
    x-axis Low Technical Accuracy --> High Technical Accuracy (Zero False Positives)
    y-axis Cloud Only / Leaky Egress --> Sovereign / Air-Gapped / High Performance
    quadrant-1 ScanDrix Enterprise (Market Leader)
    quadrant-2 Niche Local Linters
    quadrant-3 Legacy Script Bots (Kodus, CodeRabbit)
    quadrant-4 Traditional Cloud SAST (SonarQube, Snyk)
    "ScanDrix Enterprise": [0.92, 0.94]
    "Kodus AI (Legacy)": [0.45, 0.32]
    "CodeRabbit": [0.55, 0.28]
    "SonarQube": [0.70, 0.65]
    "GitHub Copilot PR": [0.60, 0.35]
```

1. **Sandboxed Verification vs. Hallucinated Suggestions:** Legacy bots generate raw markdown with missing imports or syntax errors. ScanDrix executes suggestions in microVMs to guarantee they compile and pass unit tests.
2. **Multi-Agent Deliberation vs. Single Prompt:** Legacy tools use single-pass prompt templates. ScanDrix uses specialized parallel agents that critique and cross-examine each other.
3. **Hardware-Signed Offline Licensing vs. HTTP Ping:** ScanDrix uses Ed25519 asymmetric cryptography allowing true offline validation without outbound network pings.
4. **Language Architecture:** Go compiled binaries vs. Node.js/NestJS interpreted event loops (50x lower RAM, instant startup, multi-core parallelism).

---

## 4. Epic & Feature Requirements

### Epic 1: Autonomous Multi-Agent Deliberation Council
* **REQ-1.1:** The engine must spawn three parallel specialized analysis agents for every eligible pull request diff:
  * *Lead Architect Agent:* Focuses on OOP/SOLID/FP design, modularity, idiomatic conventions, and readability.
  * *AppSec Red-Team Agent:* Analyzes CWE/OWASP vulnerabilities, tainted user inputs, improper authorization, encryption flaws, and secret exposure.
  * *Performance & Reliability Agent:* Identifies memory leaks, unclosed streams/goroutines, N+1 queries, race conditions, and quadratic time complexity.
* **REQ-1.2:** Findings from all three agents must be piped to the **Arbiter Judge Agent**, which performs deduplication, weighs severity, discards subjective nits, and requires a minimum **confidence threshold of 92%** before accepting an issue.
* **REQ-1.3:** The final review comment must render a structured, clean executive summary table, highlighting categorized issues with rationale and verifiable patches.

### Epic 2: Sandboxed Suggestion Execution & Verification (MicroVM)
* **REQ-2.1:** Every code suggestion generated by ScanDrix must be validated through an isolated execution environment (Docker container, Firecracker microVM, or E2B sandbox).
* **REQ-2.2:** The sandbox must apply the proposed `git diff` patch on the base branch commit and execute the project’s native linter and build command (e.g., `go test -c`, `cargo check`, `npm run build`, `pytest`).
* **REQ-2.3:** If the sandbox reports a compilation or test failure, the error log must be fed back to the LLM for self-correction (max 2 repair iterations). If it still fails, the suggestion is suppressed to maintain zero false positives.

### Epic 3: Semantic AST Call-Graph Impact Analysis
* **REQ-3.1:** Integrated Tree-sitter parser in Go supporting 12+ primary programming languages (Go, TypeScript, JavaScript, Python, Rust, Java, C#, C++, Ruby, PHP, Kotlin, Swift).
* **REQ-3.2:** When a function, method, or class signature is modified, the engine must extract the symbol name and scan the repository index to identify all call sites across unchanged files.
* **REQ-3.3:** The PR review must explicitly alert engineers to breaking downstream changes (e.g., "Function `CalculateDiscount` modified: 4 call sites in `billing/` may experience runtime errors").

### Epic 4: Sovereign Air-Gapped Operation & BYOK
* **REQ-4.1:** The system must function completely disconnected from external internet access when configured in `AIR_GAPPED=true` mode.
* **REQ-4.2:** Native support for local inference endpoints using standard OpenAI/Anthropic wire protocols (vLLM, Ollama, TGI, LocalAI, DeepSeek-R1, Llama 3.3).
* **REQ-4.3:** Bring-Your-Own-Key (BYOK) encryption: Enterprise customer API keys are encrypted at rest using AES-256-GCM with tenant-isolated KMS keyrings. Keys are decrypted strictly in volatile RAM during active inference calls.

### Epic 5: Enterprise Identity & Directory Provisioning (SAML & SCIM)
* **REQ-5.1:** SAML 2.0 Single Sign-On supporting Okta, Microsoft Entra ID (Azure AD), Google Workspace, PingFederate, and Keycloak with cryptographic X.509 signature verification.
* **REQ-5.2:** SCIM 2.0 Server (`/scim/v2/Users`, `/scim/v2/Groups`) to automate real-time user provisioning, role synchronization, and deprovisioning when employees join or leave the organization.
* **REQ-5.3:** Role-Based Access Control (RBAC) with four enterprise tiers: `Owner`, `Security_Admin`, `Engineering_Lead`, and `Developer`.

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
  * `Tier` (Community, Developer, Team, Enterprise)
  * `ExpiresAt` (Timestamp)
  * `MaxSeats` (Developer seat cap)
  * `MaxRepositories` (Linked repo cap)
  * `Features` (Bitmask flags: `BYOK`, `AIR_GAPPED`, `MULTI_AGENT`, `SCIM`, `DORA_METRICS`)
  * `HardwareFingerprint` (Optional server UUID lock)
* **REQ-7.3:** Sub-millisecond license verification with graceful 7-day grace period warnings before hard gating.

---

## 5. Non-Functional Requirements (NFRs)

### 5.1 Performance & Latency
* **PR Review Throughput:** P50 < 20 seconds, P95 < 45 seconds for pull requests up to 50 files and 2,500 lines of diff.
* **Memory Efficiency:** ScanDrix worker processes must idle under **75 MB RSS** and peak under **350 MB RSS** under heavy multi-agent AST load.
* **Database Query Performance:** 99% of analytical dashboard queries must resolve in **< 120ms** across 1,000,000+ historical review runs.

### 5.2 Reliability & Fault Tolerance
* **System Uptime:** Designed for 99.99% availability in clustered configurations.
* **Message Delivery Guarantee:** Zero message loss via RabbitMQ Quorum queues with delayed retry exchanges and dead-letter queues (max 5 backoff retries).
* **Idempotency:** Webhook processing must use distributed Redis/PostgreSQL locks (`x-claim-key`) ensuring duplicate Git provider webhooks never execute twice.

### 5.3 Security & Compliance
* **SOC 2 Type II & ISO 27001 Alignment:** Immutable audit logs with RFC 3161 timestamps capturing all administrative actions, policy modifications, and SSO logins.
* **Zero Data Retention Option:** Configurable flag allowing enterprises to disable all code snippet persistence in the database; only aggregated metrics and cryptographic hashes are stored.
* **Least Privilege Isolation:** Native PostgreSQL Row-Level Security (RLS) enforcing tenant isolation at the database kernel level (`app.current_tenant_id`).

---

## 6. Success Metrics & KPIs

```
┌──────────────────────────────────────────────────────────┐
│              Enterprise Success Scorecard                │
├──────────────────────────────┬───────────────────────────┤
│ Metric                       │ Enterprise Target         │
├──────────────────────────────┼───────────────────────────┤
│ PR Suggestion Acceptance Rate│ > 68% merged without edit │
│ False Positive Rate          │ < 4% reported false alarm │
│ Review Latency (P90)         │ < 45 seconds              │
│ Security Vulnerability Catch │ > 94% CWE pre-merge       │
│ Air-Gapped Egress Leaks      │ Exactly 0 bytes           │
│ License Check Overhead       │ < 15 microseconds         │
└──────────────────────────────┴───────────────────────────┘
```
