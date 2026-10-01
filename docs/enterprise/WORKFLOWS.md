# ScanDrix Enterprise Edition — Visual Workflows & Architecture
**Version:** 2.0 Enterprise  
**Status:** Canonical Visual Reference  
**Audience:** System Architects, Staff Engineers, Enterprise DevOps  
**Confidentiality:** Proprietary & Confidential — ScanDrix  

---

## 1. End-to-End Pull Request Review Lifecycle

This diagram demonstrates the lifecycle of an incoming pull request from initial webhook trigger to verified review comment publication on GitHub, GitLab, Bitbucket, or Azure DevOps.

```mermaid
flowchart TD
    A([Git SCM Webhook: PR Opened / Updated]) --> B[cmd/webhooks: Webhook Gateway]
    
    subgraph Ingestion_Gate ["1. Ingestion & Deduplication"]
        B --> C{Verify HMAC Webhook Secret}
        C -- Invalid --> D[Drop & Return 401 Unauthorized]
        C -- Valid --> E[Generate Idempotency Key: repo_id:pr_num:head_sha]
        E --> F{Redis Distributed Lock Acquired?}
        F -- No: Duplicate In-Flight --> G[Acknowledge 200 OK & Exit]
        F -- Yes: First Event --> H[Publish review.enqueue to RabbitMQ Quorum Queue]
    end

    subgraph Worker_Processing ["2. Go Worker Processing Engine"]
        H --> I[cmd/worker: Consume Job from Quorum Queue]
        I --> J[Fetch Git Diff & Base Tree via SCM API]
        J --> K[Tree-sitter AST Parser: Language-Specific Concrete Syntax Trees]
        K --> L[Cross-File Symbol Resolver: Identify Downstream Call Sites]
        L --> M[Assemble Context Bundle: Diff + AST + Downstream Calls + Custom Rules]
    end

    subgraph Deliberation_Council ["3. Adversarial Multi-Agent Council"]
        M --> N1[Agent 1: Software Architect]
        M --> N2[Agent 2: AppSec Red Team]
        M --> N3[Agent 3: Performance & Concurrency]
        
        N1 --> O[Arbiter Judge: Cross-Examination & Consolidation]
        N2 --> O
        N3 --> O
        
        O --> P{Confidence >= 92%?}
        P -- No: Nitpick / Low Impact --> Q[Suppress & Discard Finding]
        P -- Yes: High-Impact Issue --> R[Generate Candidate Replacement Patch]
    end

    subgraph Verification_Sandbox ["4. Isolated Sandbox Execution"]
        R --> S[Mount Patch into MicroVM / Container Sandbox]
        S --> T[Run Project Build: go test / npm test / cargo check]
        T --> U{Compilation & Tests Pass?}
        U -- Pass --> V[Tag Finding: VERIFIED_WORKING]
        U -- Fail --> W[Feed Compiler Error Back to Arbiter]
        W --> X{Attempt < 2?}
        X -- Yes --> R
        X -- No --> Q
    end

    subgraph SCM_Dispatch ["5. Publication & Audit Persistence"]
        V --> Y[Format Unified Review Comment & SARIF 2.1.0 Artifact]
        Y --> Z[Post Inline Review Comments to SCM Pull Request]
        Y --> AA[Record Metrics & Event to PostgreSQL Partitioned Warehouse]
        AA --> AB[Release Redis Distributed Lock & Ack RabbitMQ Message]
    end
```

---

## 2. Adversarial Multi-Agent Deliberation Council

The Deliberation Council eliminates false-positive noise by requiring specialized agents to critique the pull request from distinct engineering angles before an Arbiter Judge makes the final ruling.

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Developer (Pull Request)
    participant Core as ScanDrix Go Core
    participant Arch as Lead Architect Agent
    participant Sec as AppSec Red-Team Agent
    participant Perf as Performance Agent
    participant Judge as Arbiter Judge
    participant SCM as Git SCM (GitHub/GitLab)

    Dev->>SCM: Opens PR with Code Changes
    SCM->>Core: Dispatches Webhook
    Note over Core: AST & Call-Graph Context Extracted

    par Parallel Deliberation
        Core->>Arch: Evaluate (SOLID, Naming, Modularity, Abstractions)
        Core->>Sec: Evaluate (OWASP Top 10, CWEs, Taint Analysis, Secrets)
        Core->>Perf: Evaluate (Memory Leaks, Goroutines, N+1 Queries, Complexity)
    end

    Arch-->>Core: 4 Modularity Findings
    Sec-->>Core: 2 Security Warnings (1 SQLi, 1 Hardcoded Token)
    Perf-->>Core: 1 Bottleneck (O(N^2) Nested Loop)

    Core->>Judge: Submit 7 Candidate Findings for Cross-Examination
    Note over Judge: Cross-examines findings against codebase conventions.<br/>Filters out 3 subjective stylistic nitpicks.<br/>Requires Confidence >= 0.92.<br/>Approves 4 verified critical findings.

    Judge-->>Core: 4 Approved Suggestions + Unified Diffs
    Core->>SCM: Post Inline Suggestions with Verified Diffs
    SCM-->>Dev: Developer Receives 4 Clean, High-Impact Actionable Reviews
```

---

## 3. Sandboxed Suggestion Verification Loop

ScanDrix is the only platform that guarantees suggestions compile and pass tests by executing them in a microVM sandbox prior to posting.

```mermaid
stateDiagram-v2
    [*] --> CandidatePatchGenerated: Arbiter generates proposed fix
    
    state "Sandbox Container Lifecycle" as Sandbox {
        CandidatePatchGenerated --> SpawnSandbox: Spin up rootless container
        SpawnSandbox --> MountCommit: Checkout PR base commit on tmpfs
        MountCommit --> ApplyPatch: Run 'git apply proposed_fix.diff'
        
        state CompileCheck <<choice>>
        ApplyPatch --> RunCompiler: Execute project build command
        RunCompiler --> CompileCheck
        
        CompileCheck --> RunUnitTests: Build Clean (Exit 0)
        CompileCheck --> BuildFailed: Build Error (Exit 1)
        
        state TestCheck <<choice>>
        RunUnitTests --> TestCheck
        TestCheck --> SandboxPassed: All Tests Pass (Exit 0)
        TestCheck --> TestsFailed: Test Regression (Exit 1)
    }

    state "Self-Correction Feedback Loop" as RepairLoop {
        BuildFailed --> CaptureDiagnostics: Extract compiler error log
        TestsFailed --> CaptureDiagnostics: Extract failed assertion diff
        CaptureDiagnostics --> PromptRepair: Pass logs back to LLM Arbiter
        
        state RetryCheck <<choice>>
        PromptRepair --> RetryCheck
        RetryCheck --> CandidatePatchGenerated: Attempt <= 2
        RetryCheck --> SuppressSuggestion: Attempt > 2 (Suppress to avoid broken code)
    }

    SandboxPassed --> PostToPR: Attach 'Verified by ScanDrix Sandbox' badge
    SuppressSuggestion --> [*]: Discarded (Zero noise)
    PostToPR --> [*]: Completed
```

---

## 4. Air-Gapped Sovereign AI & BYOK Data Flow

For defense, banking, healthcare, and air-gapped enterprise environments, ScanDrix guarantees **zero data leaves the customer perimeter**.

```mermaid
flowchart LR
    subgraph Enterprise_Firewall ["Customer On-Premise / VPC Air-Gapped Boundary"]
        subgraph Vault_Layer ["Tenant Key Vault"]
            KMS[(Customer AWS KMS / HashiCorp Vault)]
            ENC_KEY[Encrypted BYOK Key at Rest]
        end

        subgraph Core_Engine ["ScanDrix Enterprise Core"]
            MEM[Volatile RAM: Decrypted for Active Request Only]
            ENGINE[ScanDrix Go Worker Engine]
            AST_LOCAL[Tree-sitter AST & Heuristic Engine]
        end

        subgraph Local_Inference ["Sovereign Local Inference Cluster"]
            VLLM[vLLM / Ollama Cluster]
            MODELS[(DeepSeek-R1 / Qwen 2.5 Coder / Llama 3.3)]
        end
    end

    subgraph Public_Internet ["Public Cloud (BLOCKED / ISOLATED)"]
        PUBLIC_AI[Public OpenAI / Anthropic APIs]
    end

    ENC_KEY <-->|AES-256-GCM Decrypt via KMS| KMS
    KMS --> MEM
    ENGINE --> AST_LOCAL
    ENGINE <-->|OpenAI Protocol over Private VPC Network| VLLM
    VLLM <--> MODELS
    
    ENGINE x-.-x|STRICTLY BLOCKED: Zero Egress Rule| PUBLIC_AI

    classDef blocked fill:#ffcccc,stroke:#ff0000,stroke-width:2px;
    class PUBLIC_AI blocked;
```

---

## 5. Enterprise SCIM 2.0 Directory Lifecycle Flow

Automated developer seat provisioning, role mapping, and instant deprovisioning synchronized directly with enterprise identity providers.

```mermaid
sequenceDiagram
    autonumber
    participant IdP as Identity Provider (Okta / Entra ID)
    participant SCIM as ScanDrix SCIM 2.0 Gateway (/scim/v2)
    participant DB as PostgreSQL Core Database
    participant License as Ed25519 Seat Tracker

    Note over IdP: Employee joins Engineering Team
    IdP->>SCIM: POST /scim/v2/Users (Marcus Vance, role=ENGINEERING_LEAD)
    SCIM->>License: Check Available Seat Quota
    alt Quota Available
        License-->>SCIM: Quota OK (Seat 42 of 100)
        SCIM->>DB: Insert User & Assign Workspace Role
        SCIM-->>IdP: 201 Created (SCIM User Resource)
    else Quota Exceeded
        License-->>SCIM: Quota Exhausted (100 of 100 Seats in use)
        SCIM-->>IdP: 409 Conflict ("License seat limit reached")
    end

    Note over IdP: Employee leaves company
    IdP->>SCIM: PATCH /scim/v2/Users/{id} {"active": false}
    SCIM->>DB: Set user.status = 'SUSPENDED'
    SCIM->>License: Release Allocated Seat (Now 41 of 100)
    SCIM-->>IdP: 200 OK (User Deactivated)
```

---

## 6. Offline Asymmetric Cryptographic License Check

Sub-millisecond license entitlement validation without internet access.

```mermaid
flowchart TD
    A[Customer Boots ScanDrix Enterprise Node] --> B[Read SCANDRIX_LICENSE_TOKEN from Environment / Mount]
    B --> C[Split Token: Base64 Payload . Base64 Signature]
    C --> D[Load Compile-Time Embedded ScanDrix Authority Public Key]
    
    D --> E[Call ed25519.Verify pubKey, rawPayload, rawSignature]
    E --> F{Signature Valid?}
    
    F -- No / Tampered --> G[Fatal Panic: 'Invalid Cryptographic Signature. License Corrupt.']
    F -- Yes --> H[Unmarshal License Claims JSON]
    
    H --> I{time.Now UTC > ExpiresAt?}
    I -- Yes: Expired --> J{Within 7-Day Grace Period?}
    J -- Yes --> K[Log Warning: 'License Expired. 7-Day Grace Window Active.']
    J -- No --> L[Lock Engine: 'License Expired. Contact legal@scandrix.dev']
    
    I -- No: Active --> M[Verify Optional Hardware / Cluster UUID Fingerprint]
    M --> N{Hardware Matches?}
    N -- No --> O[Fatal Panic: 'License bound to different hardware UUID']
    N -- Yes --> P[Enable Enterprise Feature Bitmasks: BYOK, AIR_GAPPED, MULTI_AGENT, SCIM]
    P --> Q[Start Go Worker Daemons & Chi REST Endpoints]
```

---

## 7. DORA Metrics & Engineering Intelligence Ingestion

```mermaid
flowchart LR
    subgraph Event_Sources ["Continuous Lifecycle Events"]
        PR_M[PR Merged Event]
        PR_C[PR Created Event]
        CI_F[CI Deployment Failure]
        REV_T[Review Completed Event]
    end

    subgraph Ingestion_Stream ["Async Aggregation Pipeline"]
        PR_M --> WRK[ScanDrix Analytics Ingestion Worker]
        PR_C --> WRK
        CI_F --> WRK
        REV_T --> WRK
    end

    subgraph Partitioned_Warehouse ["PostgreSQL Time-Series Warehouse"]
        WRK --> RAW[(analytics_pull_request_events Partitioned)]
        RAW --> ROLLUP[(materialized_dora_daily_rollups)]
    end

    subgraph Executive_Dashboard ["ScanDrix Analytics UI (scandrix-website)"]
        ROLLUP --> DORA_API[GET /api/v1/analytics/dora]
        DORA_API --> UI1[Deployment Frequency Graph]
        DORA_API --> UI2[Lead Time for Changes: P50/P90]
        DORA_API --> UI3[Change Failure Rate %]
        DORA_API --> UI4[Time to Restore Service MTTR]
    end
```
