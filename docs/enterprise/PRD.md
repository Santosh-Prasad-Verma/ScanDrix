# ScanDrix Enterprise Edition — Product Requirements Document (PRD)

**Version:** 2.0 Enterprise  
**Status:** Approved & Target Architecture  
**Product:** ScanDrix AI Enterprise Code Review & Engineering Intelligence  
**Author:** ScanDrix Architecture & Product Engineering  

---

## 1. Executive Summary & Vision

### 1.1 Product Vision
**ScanDrix Enterprise** is an autonomous AI code review, application security gate, and engineering intelligence platform engineered natively in **compiled Go**. 

ScanDrix addresses enterprise software review bottlenecks by executing multi-agent deliberation across architectural, security, and performance dimensions; validating code changes in isolated sandboxes before posting; and operating under strict data residency constraints through sovereign air-gapped deployments and offline cryptographic licensing.

### 1.2 Core Capabilities
1. **Sandboxed Verification:** Proposed code suggestions are compiled, linted, and test-checked in an isolated sandbox prior to publishing. Suggestions that cannot be verified are explicitly tagged as unverified to eliminate developer noise.
2. **Adversarial Multi-Agent Deliberation:** Specialist agents (Software Architecture, AppSec Red Team, Performance & Concurrency) analyze pull request diffs concurrently under an Arbiter Judge that consolidates findings and suppresses low-confidence commentary.
3. **Sovereign & Air-Gapped Deployments (BYOK):** Full on-premise execution with local LLMs (DeepSeek, vLLM, Ollama, Llama) with strict network egress gating, zero-data-retention compliance, and client-managed encryption key rings.
4. **Compiled High-Performance Engine:** Native Go static binaries, horizontal scaling over RabbitMQ quorum queues, PostgreSQL 16+ row-level security, and sub-millisecond cryptographic license checks.

---

## 2. Target Personas & Stakeholders

| Persona | Role | Primary Objectives |
| :--- | :--- | :--- |
| **Engineering Leadership** | VP of Engineering / CTO | Reduce PR review cycle times, eliminate reviewer fatigue, track objective DORA velocity metrics, and maintain engineering standards. |
| **AppSec & Compliance** | CISO / Security Director | Prevent secret leaks, identify CWE/OWASP vulnerabilities before merge, enforce license compliance, and guarantee zero source-code retention. |
| **Staff & Principal Engineers** | Technical Architects | Enforce domain architecture, prevent anti-patterns and performance regressions, and maintain consistent idioms across repositories. |
| **Software Engineers** | Core Contributors | Receive actionable, compiling code improvements that can be committed directly from the Git provider interface. |

---

## 3. Architecture & Product Pillars

1. **Deterministic Suggestion Verification:** Code review suggestions are executed against the repository's build toolchain in ephemeral micro-containers before posting.
2. **Multi-Agent Deliberation Council:** Specialized agents evaluate pull requests in parallel across design, security, and performance domains, preventing single-model blind spots.
3. **Cryptographically Signed Offline Licensing:** High-security Ed25519 asymmetric signature verification requiring zero phone-home network calls.
4. **Zero-Trust Data Protection:** Tenant-isolated PostgreSQL Row-Level Security (RLS) coupled with Bring-Your-Own-Key (BYOK) AES-256-GCM encryption.

---

## 4. Feature Specifications

### Epic 1: Multi-Agent Deliberation Council
* **REQ-1.1 (Specialist Evaluation):** For every eligible pull request diff, ScanDrix dispatches three specialized review agents:
  * *Software Architect Agent:* Evaluates modularity, SOLID principles, idiomatic patterns, interface abstraction, and maintainability.
  * *AppSec Red-Team Agent:* Analyzes OWASP Top 10, tainted input propagation, authorization boundaries, cryptographic hygiene, and credential leaks.
  * *Performance & Reliability Agent:* Detects memory leaks, unclosed resources, race conditions, N+1 query patterns, and algorithmic bottlenecks.
* **REQ-1.2 (Arbiter Judge):** Consolidates findings from all specialist agents, deduplicates overlapping issues, calculates composite severity, and filters out subjective nits.
* **REQ-1.3 (Structured Delivery):** Generates structured markdown summaries, file-level inline suggestions with committable diff blocks, and SARIF 2.1.0 output for CI/CD integration.

### Epic 2: Sandboxed Verification Engine
* **REQ-2.1 (Ephemeral Execution):** Every proposed code patch is applied to a clone of the base commit inside an isolated container sandbox.
* **REQ-2.2 (Build & Test Validation):** The sandbox executes the repository's build and verification commands (e.g. `go test -c`, `cargo check`, `npm test`, `pytest`).
* **REQ-2.3 (Self-Correction Loop):** Compilation or test errors are passed back to the engine for automated refinement (up to 2 iterations). If errors persist, the suggestion is published with an unverified label or suppressed per repository policy.

### Epic 3: Call-Graph Impact Analysis
* **REQ-3.1 (Multi-Language AST Parsing):** Pure-Go concrete syntax parsing supporting Go, TypeScript, JavaScript, Python, Rust, Java, and C/C++.
* **REQ-3.2 (Cross-File Symbol Resolution):** When exported function, struct, or interface signatures are modified, the engine resolves call sites and downstream dependencies across unchanged repository files.
* **REQ-3.3 (Breaking Change Alerts):** Highlights potential breaking API changes and lists affected downstream modules directly within the review summary.

### Epic 4: Sovereign Air-Gapped & BYOK Infrastructure
* **REQ-4.1 (Air-Gapped Operation):** Complete functionality in isolated, internet-disconnected VPC environments.
* **REQ-4.2 (Local Model Support):** Seamless compatibility with local OpenAI-compatible inference servers including vLLM, Ollama, DeepSeek, and TGI.
* **REQ-4.3 (BYOK Key Management):** Client API keys are encrypted at rest using AES-256-GCM with tenant-isolated keys and decrypted in volatile memory only during active inference.

### Epic 5: Enterprise Identity & Provisioning
* **REQ-5.1 (SAML 2.0 / OIDC):** Single Sign-On integration supporting Okta, Microsoft Entra ID, Google Workspace, Keycloak, and PingFederate.
* **REQ-5.2 (SCIM 2.0 Directory Sync):** Automated user provisioning and deprovisioning via standard `/scim/v2/Users` and `/scim/v2/Groups` endpoints.
* **REQ-5.3 (Role-Based Access Control):** Granular permissions partitioned across `OWNER`, `ADMIN`, `MEMBER`, and `VIEWER` roles enforced via database-backed policies.
* **REQ-5.4 (Multi-Factor Authentication):** Enforceable TOTP authentication with secure recovery codes and login rate limiting.

### Epic 6: Engineering Intelligence & DORA Metrics Warehouse
* **REQ-6.1 (Event Ingestion):** Captures pull request lifecycle events into a PostgreSQL time-partitioned analytics warehouse.
* **REQ-6.2 (DORA Velocity Computation):** Automated calculation of Deployment Frequency, Lead Time for Changes, Change Failure Rate, and Mean Time to Restore.
* **REQ-6.3 (Code Quality Indicators):** Metrics for Review Turnaround Time, Suggestion Acceptance Rate, and Recurring Bug Hotspots.

### Epic 7: Asymmetric Cryptographic Licensing
* **REQ-7.1 (Offline Ed25519 Signatures):** Offline license validation via public-key cryptography.
* **REQ-7.2 (License Envelope Entitlements):** Signed license envelopes encode customer identity, subscription tier, expiration timestamp, seat quotas, and feature flags (`FEATURE_BYOK_ENCRYPTION`, `FEATURE_AIR_GAPPED`, `FEATURE_MULTI_AGENT_DELIBERATION`, `FEATURE_DORA_METRICS`).
* **REQ-7.3 (Grace Period & Key Rotation):** 7-day grace window for license renewals with support for overlapping signing key rotation rings.

---

## 5. Non-Functional Requirements (NFRs)

### 5.1 Performance & Scalability
* **Review Latency:** P50 turnaround under 25 seconds; P95 under 45 seconds for pull requests under 50 files / 2,500 lines of diff.
* **Memory Footprint:** Worker process RSS < 350 MB under concurrent load; idle baseline < 150 MB.
* **Database Efficiency:** Dashboard query latency P99 < 120ms utilizing daily materialized rollups and tenant-partitioned indexes.

### 5.2 Reliability & Fault Tolerance
* **Availability Target:** 99.9% for single-node installations; 99.95% for clustered high-availability deployments.
* **Queue Durability:** RabbitMQ quorum queues with persistent messaging and dead-letter queue (DLQ) exponential backoff retries.
* **Idempotent Ingestion:** Webhook deduplication using distributed Redis locks preventing duplicate pipeline runs for identical commit events.

### 5.3 Security & Data Isolation
* **Row-Level Security (RLS):** Native PostgreSQL kernel isolation enforcing strict workspace data segregation on all queries.
* **Zero Data Retention:** Optional mode ensuring zero persistent storage of source code or diff payloads.
* **Audit Trails:** Immutable, hash-chained audit logging recording all administrative and authentication events.
