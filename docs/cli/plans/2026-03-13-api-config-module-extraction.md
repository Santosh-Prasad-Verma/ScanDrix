# API Config Module Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract the HTTP core and repository-config API client out of `src/services/api/api.real.ts` while preserving the current `RealApi` surface and behavior.

**Architecture:** Move generic request/retry/config-cache utilities into a shared API core module, move `RealConfigApi` into its own file with lightweight requester injection for isolated tests, and keep `RealApi` as the assembly point for domain-specific API clients. Preserve endpoint behavior and existing tests.

**Tech Stack:** TypeScript, Vitest, Fetch API

### Task 1: Pin the new config API module

**Files:**
- Create: `src/services/api/__tests__/config.api.test.ts`

**Step 1: Write the failing test**

Add focused tests for a dedicated `RealConfigApi` class that verify:
- it sends `X-Team-Key` for team-key access
- it sends `Authorization` for bearer access
- it targets the expected repository settings endpoint

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/config.api.test.ts`

Expected: FAIL because the new config API module does not exist yet.

### Task 2: Extract API core and config client

**Files:**
- Create: `src/services/api/api-core.ts`
- Create: `src/services/api/config.api.ts`
- Modify: `src/services/api/api.real.ts`

**Step 1: Move shared request logic**

Extract config cache, API base URL resolution, Cloudflare headers, API error normalization, `request`, and `requestWithRetry` into `api-core.ts`.

**Step 2: Move RealConfigApi**

Create `config.api.ts` with a dedicated `RealConfigApi` class that depends on the shared request helper.

**Step 3: Reassemble RealApi**

Update `api.real.ts` to import `RealConfigApi` and the shared core exports, keeping `RealApi` public behavior unchanged.

### Task 3: Verify behavior stayed stable

**Files:**
- Test: `src/services/api/__tests__/config.api.test.ts`
- Test: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/services/api/__tests__/config.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: PASS.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/services/api/api-core.ts src/services/api/config.api.ts src/services/api/api.real.ts src/services/api/__tests__/config.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
