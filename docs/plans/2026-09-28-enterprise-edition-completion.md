# ScanDrix Enterprise Edition Completion — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement all remaining Enterprise Edition (EE) capabilities specced across `PRD.md`, `TRD.md`, `WORKFLOWS.md`, and `IMPLEMENTATION.md`: (1) Partitioned DORA Metrics Warehouse Migration 038 & Repository, (2) Pure-Go Multi-Language Call-Graph Impact Analysis Engine (`ImpactAnalyzer`), (3) Sandboxed 2-Pass Compiler & Test Suggestion Verification Loop, (4) Sovereign Air-Gapped Egress Firewall Enforcement Gate, and (5) Documentation Synchronization.

**Architecture:** Extend existing ScanDrix subsystems without duplicate parallel trees. Adhere strictly to the MASTER RULES: zero fake data/mock stubs, derived metrics report absence (`unavailable`) rather than fabricated constants or zeros, parameterized SQL queries with `scandrix_runtime` least-privilege RLS, and secrets loaded strictly from environment/vault.

**Tech Stack:** Go 1.25, PostgreSQL 15+ (monthly range partitioning, RLS, pgx/v5), crypto/sha256, pure-Go AST parsers, E2B/local sandboxes.

---

### Task 1: Migration 038 & Partitioned DORA Metrics Warehouse (Phase 5, Epic 6)

**Files:**
- Create: `migrations/038_partitioned_analytics_dora.sql`
- Create: `internal/database/dora_repository.go`
- Create: `internal/database/dora_repository_test.go`
- Modify: `internal/cron/dora_aggregator.go`

**Step 1: Write migration `038_partitioned_analytics_dora.sql`**
- DDL for `analytics_pull_request_events` partitioned by range (`created_at`) with monthly partitions for 2026.
- DDL for `materialized_dora_daily_rollups` with primary key `(workspace_id, rollup_date)`.
- RLS enabled on both tables with `scandrix_runtime` tenant isolation (`workspace_id = app.current_tenant_id()`).

**Step 2: Write failing unit test for DORA repository**
- `TestDORARepository_RollupAndQuery`: Seed real PR events and test rollup calculation.
- `TestDORARepository_AbsenceWhenNoData`: Master Rule 2.7 compliance — assert that missing metrics return `nil` with machine-readable `unavailable` reasons (`insufficient_data`).

**Step 3: Implement `internal/database/dora_repository.go`**
- Implement `AggregateDORARollup` and `GetDORAMetrics(workspaceID, window)`.
- Wire `dora_aggregator.go` to call real calculation.

**Step 4: Run tests and apply migration to local & Supabase databases**
- Execute tests to ensure 100% pass rate.
- Run migration against local PostgreSQL (`scandrix-postgres`) and Supabase.

---

### Task 2: Pure-Go Multi-Language Call-Graph Impact Analysis Engine (Phase 2, Epic 3)

**Files:**
- Create: `internal/codeanalysis/callgraph/models.go`
- Create: `internal/codeanalysis/callgraph/impact_analyzer.go`
- Create: `internal/codeanalysis/callgraph/impact_analyzer_test.go`
- Modify: `internal/review/orchestrator/types.go`

**Step 1: Write failing test in `internal/codeanalysis/callgraph/impact_analyzer_test.go`**
- Test parsing diff hunks, identifying modified symbols, and tracing caller sites in unchanged files.
- Test breaking signature changes (parameter count / type changes).

**Step 2: Implement `models.go` and `impact_analyzer.go`**
- Implement exact contract: `SymbolChange`, `CallSite`, `ImpactReport`, `ImpactAnalyzer`.
- High-efficiency diff hunk parser + AST symbol matching without CGO.

**Step 3: Run tests and verify performance**
- Run `go test -v ./internal/codeanalysis/callgraph/...`.

---

### Task 3: Sandboxed Automated 2-Pass Suggestion Compiler & Test Verification Loop (Phase 4, Epic 2)

**Files:**
- Create: `internal/sandbox/verifier/verifier.go`
- Create: `internal/sandbox/verifier/verifier_test.go`
- Create: `internal/review/pipeline/stages/verify_suggestions.go`
- Create: `internal/review/pipeline/stages/verify_suggestions_test.go`

**Step 1: Write failing test in `internal/sandbox/verifier/verifier_test.go`**
- Test patch application in sandbox.
- Test compilation check (`go test -c`, etc.).
- Test 2-pass repair feedback loop: simulate compilation failure, error fed back to self-correction function, fixed patch passes on iteration 2.
- Test fail-closed degradation: when 2 attempts fail, flag as unverified (no silent pass).

**Step 2: Implement `verifier.go`**
- Implement `SuggestionVerifier` with `VerifyAndRepair(ctx, sandbox, finding, repairFn)`.

**Step 3: Implement pipeline stage `verify_suggestions.go`**
- Connect into review pipeline after agent deliberation.

**Step 4: Run tests**
- `go test -v ./internal/sandbox/verifier/... ./internal/review/pipeline/stages/... -run Suggestion`.

---

### Task 4: Air-Gapped Egress Firewall Runtime Enforcement Gate (Phase 1, Epic 4)

**Files:**
- Create: `internal/platform/security/airgap.go`
- Create: `internal/platform/security/airgap_test.go`
- Modify: `internal/llm/gateway.go`

**Step 1: Write failing test in `internal/platform/security/airgap_test.go`**
- Test blocking outbound requests to public IPs / cloud LLM endpoints when `AIR_GAPPED=true`.
- Test allowing private localhost and allowlisted local inference clusters (e.g. `127.0.0.1:11434`, `10.0.0.0/8`).
- Test CEF audit log generation on blocked egress.

**Step 2: Implement `airgap.go`**
- Implement `AirGappedTransport` implementing `http.RoundTripper`.

**Step 3: Wire into HTTP client in `internal/llm/gateway.go`**
- Run `go test -v ./internal/platform/security/... ./internal/llm/...`.

---

### Task 5: Documentation & Workflow Synchronization

**Files:**
- Modify: `docs/enterprise/WORKFLOWS.md`
- Modify: `docs/enterprise/IMPLEMENTATION.md`

**Step 1: Update WORKFLOWS.md**
- Update Flow 5 status to `[IMPLEMENTED]`.
- Update Flow 3 with the 2-pass sandbox verification loop details.

**Step 2: Update IMPLEMENTATION.md**
- Mark completed milestones with exact file references and test verification results.
