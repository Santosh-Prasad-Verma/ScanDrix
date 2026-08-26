# System Architecture & Technical Design — CodeHound (ForgeGuard)

**Document:** 03-System-Architecture.md  
**Status:** Approved Master Architecture  
**Target:** Distributed Control Plane, Temporal Workflows, Firecracker MicroVMs & Data Pipelines  
**Date:** 2026-08-25  

---

## 1. High-Level System Context & Ingress

CodeHound is an autonomous engineering platform designed to process codebase snapshots, run deterministic analysis, orchestrate multi-model reasoning, execute sandboxed QA, run authorized DAST/load surges, and generate verified auto-remediation patches.

```mermaid
flowchart TB
    subgraph Clients["1. Universal Client Ingress"]
        CLI["Terminal CLI / TUI (Go Binary)"]
        WEB["Web Console (SvelteKit / React)"]
        IDE["IDE Extensions (VS Code / JetBrains / Cursor LSP)"]
        MCP["MCP Server (Stateless 2026 Protocol)"]
        GH["GitHub / GitLab / Bitbucket Apps (Webhooks)"]
    end

    subgraph Edge["2. Edge & Security Gateway"]
        WAF["Cloudflare Edge & WAF"]
        APIGW["Unified API Gateway & Ingress Router (Go)"]
        AUTH["Identity & Scope Validator (OIDC / PAT / HMAC)"]
        SSE["SSE & WebSocket Streaming Hub"]
    end

    subgraph ControlPlane["3. Core Control Plane & Orchestration"]
        API_SVC["Control API Service"]
        POLICY_ENG["Policy & Compliance Engine"]
        TARGET_AUTH["Target Cryptographic Auth Service"]
        TEMPORAL["Temporal Workflow Orchestration Cluster (Durable State)"]
        NATS["NATS JetStream (High-Throughput Event Bus)"]
    end

    subgraph Workers["4. Specialized Worker Fleets"]
        INGEST_W["Ingestion & AST Workers (Tree-sitter)"]
        SAST_W["Static Scanners (Semgrep, Gitleaks, TruffleHog, Syft)"]
        AI_ROUTER["Multi-Model Router (Triage, Reasoning, Arbiter)"]
        VERIFY_W["Verification & QA Engine"]
    end

    subgraph Sandboxes["5. Isolated Execution Plane"]
        FC_POOL["Bare-Metal Firecracker MicroVM Pool (KVM)"]
        DAST_LAB["Dynamic Security Lab (OWASP ZAP / Nuclei)"]
        K6_FLEET["Distributed k6 Load Runners (1k -> 10k -> 50k VUs)"]
    end

    subgraph DataStorage["6. Storage & State Layer"]
        PG[(Amazon Aurora PostgreSQL 18 HA / RDS - Relational & Graph)]
        ASTRA[(DataStax Astra DB - Serverless Vector JSON API)]
        S3[(Amazon S3 / MinIO - Immutable Snapshots & Reports)]
    end

    Clients --> WAF --> APIGW --> AUTH --> API_SVC
    API_SVC --> POLICY_ENG & TARGET_AUTH
    API_SVC --> TEMPORAL
    TEMPORAL --> NATS
    NATS --> INGEST_W & SAST_W & AI_ROUTER & VERIFY_W
    
    INGEST_W --> PG & ASTRA & S3
    SAST_W --> PG & S3
    AI_ROUTER --> PG
    VERIFY_W --> FC_POOL
    TEMPORAL --> DAST_LAB & K6_FLEET

    API_SVC --> SSE
    SSE -.-> CLI & WEB & IDE
```

---

## 2. End-to-End Audit & Scan Pipeline Lifecycle

```mermaid
sequenceDiagram
    autonumber
    participant Client as Client (CLI / Web / IDE / GitHub)
    participant API as Control API Gateway
    participant Temp as Temporal Workflow Orchestrator
    participant AST as Ingest & Tree-sitter Worker
    participant SAST as Deterministic Scanners (Semgrep/Gitleaks)
    participant AI as Multi-Model Reasoning Core
    participant VM as Firecracker MicroVM Sandbox
    participant Rep as Report & Artifact Builder

    Client->>API: POST /api/v1/audits (commit_sha, policy_id)
    API->>Temp: Start Workflow: `RunCodeAuditWorkflow`
    API-->>Client: 202 Accepted (audit_id, stream_url)

    Temp->>AST: Ingest Snapshot & Extract Code Symbols/Edges
    AST->>AST: Build Call Graph, Taint Paths & Route Maps
    AST-->>Temp: Index Ready (Symbols, Edges, Astra DB Vectors)

    Temp->>SAST: Execute Semgrep + Gitleaks + TruffleHog + Syft
    SAST-->>Temp: Candidate Static Findings & Raw Evidence

    Temp->>AI: Trigger Tier-2 Dual Reasoning (Logic & Security)
    AI-->>Temp: Candidate Bugs & Synthesized Test Cases

    Temp->>VM: Launch Ephemeral MicroVM Sandbox
    VM->>VM: Execute Synthesized Tests & Run Mutation Check (Stryker)
    VM-->>Temp: Proof Evidence (Pass/Fail Traces, Mutation Score)

    Temp->>AI: Trigger Tier-3 Arbiter / Judge
    AI-->>Temp: Verified Findings + Final Confidence Score

    Temp->>Rep: Generate AUDIT_REPORT.md, SARIF v2.1.0, JUnit XML
    Rep->>API: Broadcast `audit_completed` via SSE Stream
    API-->>Client: Streamed Completion & Report Links
```

---

## 3. The Self-Healing Auto-Patch Verification Loop

CodeHound does not merely offer text suggestions; it proves fixes by running a closed-loop verification cycle:

```mermaid
flowchart TD
    VerifiedBug[Verified Bug / Vulnerability Identified] --> PatchGen[AI Patch Synthesizer: Minimal Unified Diff]
    PatchGen --> WorktreeInit[Checkout Isolated Git Worktree in MicroVM]
    WorktreeInit --> ApplyDiff[Apply Unified Diff: git apply]
    ApplyDiff --> BuildCheck{Does Code Compile Cleanly?}
    BuildCheck -- No --> FeedbackSyntax[Feed Compiler Errors to Patch Synthesizer]
    FeedbackSyntax --> PatchGen

    BuildCheck -- Yes --> RunTests[Execute Native Project Tests + Generated Tests]
    RunTests --> TestPassCheck{Do All Tests Pass?}
    TestPassCheck -- No --> FeedbackLogic[Feed Stack Trace to Patch Synthesizer]
    FeedbackLogic --> PatchGen

    TestPassCheck -- Yes --> MutationRun[Run Mutation Testing: Stryker / Mutmut]
    MutationRun --> MutationScoreCheck{Mutation Score > 80%?}
    MutationScoreCheck -- No --> FlagWeak[Flag Warning: Fix Passed but Assertions Weak]
    MutationScoreCheck -- Yes --> SecurityScan[Re-run SAST & Taint Checks on Patched Code]

    SecurityScan --> RegressCheck{Any Security Regressions?}
    RegressCheck -- Yes --> PatchGen
    RegressCheck -- No --> OpenPR[Open Verified Draft PR with Evidence Attachments]
```

---

## 4. Trust Boundaries & Network Security Tiers

| Tier | Name | Components | Network & Access Restrictions |
| :--- | :--- | :--- | :--- |
| **Tier A** | **Trusted Control Plane** | Control APIs, Temporal, Identity, PostgreSQL | Strict private VPC, TLS 1.3, IAM role isolation. |
| **Tier B** | **Semi-Trusted Workers** | AST Parsers, SARIF normalizers, LLM Gateway | Egress locked strictly to AI API providers and DB. |
| **Tier C** | **Untrusted Execution Sandbox** | Firecracker microVMs executing repository builds and tests | **Egress Deny-All** by default. Memory-backed ephemeral tmpfs. Process jailed with minimal seccomp and cgroup v2. |
| **Tier D** | **Authorized Dynamic Lab** | DAST fuzzers (ZAP/Nuclei), k6 load generators | Dedicated network egress proxy locked to cryptographic allowlist targets. Hard rate ceilings and emergency circuit breakers. |

---

## 5. Architectural Non-Functional Guarantees

1. **Deterministic Reproducibility**: Every scan is pinned to an immutable git commit content hash and policy version.
2. **Sub-120ms MicroVM Startup**: Maintained by a pre-warmed snapshot pool on bare-metal KVM worker nodes.
3. **Resilience & Durable Execution**: Temporal ensures that any worker crash or network blip during long-running 50k VU load tests or whole-repo audits resumes automatically from the exact state step without data loss.
4. **Cost-Aware AI Orchestration**: 70%+ of clean code is filtered at Tier 1 ($0.15/1M tokens), reserving expensive reasoning models ($3.00+/1M tokens) strictly for high-risk ambiguities and arbiter rulings.
