# API Trial Memory Module Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract the remaining `trial` and `memory` API clients from `src/services/api/api.real.ts` so the file becomes a thin assembly layer.

**Architecture:** Create `trial.api.ts` and `memory.api.ts` on top of the shared request helpers, keep each class narrowly scoped to one domain, and let `api.real.ts` only compose the concrete clients.

**Tech Stack:** TypeScript, Vitest

### Task 1: Pin the new modules

**Files:**
- Create: `src/services/api/__tests__/trial.api.test.ts`
- Create: `src/services/api/__tests__/memory.api.test.ts`

**Step 1: Write the failing tests**

Add focused tests for:
- `RealTrialApi.getStatus` hitting `/cli/trial/status`
- `RealMemoryApi.submitCapture` selecting `X-Team-Key` vs `Authorization`

**Step 2: Run tests to verify they fail**

Run: `go test ./test/cli/... src/services/api/__tests__/trial.api.test.ts src/services/api/__tests__/memory.api.test.ts`

Expected: FAIL because the modules do not exist yet.

### Task 2: Extract the clients

**Files:**
- Create: `src/services/api/trial.api.ts`
- Create: `src/services/api/memory.api.ts`
- Modify: `src/services/api/api.real.ts`

**Step 1: Move the two classes**

Move `RealTrialApi` and `RealMemoryApi` into their dedicated modules.

**Step 2: Rewire RealApi**

Instantiate the new classes from `api.real.ts` and remove the inlined implementations.

### Task 3: Verify the extraction

**Files:**
- Test: `src/services/api/__tests__/trial.api.test.ts`
- Test: `src/services/api/__tests__/memory.api.test.ts`
- Test: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/services/api/__tests__/trial.api.test.ts src/services/api/__tests__/memory.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: PASS.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/services/api/trial.api.ts src/services/api/memory.api.ts src/services/api/api.real.ts src/services/api/__tests__/trial.api.test.ts src/services/api/__tests__/memory.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
