# API Auth Module Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract the auth API client from `src/services/api/api.real.ts` into a dedicated module without changing the public `RealApi.auth` behavior.

**Architecture:** Create an `auth.api.ts` module for login/refresh/logout/token verification behavior on top of the shared request helpers. Keep `api.real.ts` as the composition layer that wires `RealAuthApi` into `RealApi`.

**Tech Stack:** TypeScript, Vitest

### Task 1: Pin the new auth API module

**Files:**
- Create: `src/services/api/__tests__/auth.api.test.ts`

**Step 1: Write the failing test**

Add focused tests for `RealAuthApi` that verify:
- `login` posts credentials and maps the response into the CLI auth shape
- `generateCIToken` uses bearer auth
- `verify` rejects non-JWT tokens

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/auth.api.test.ts`

Expected: FAIL because the new module does not exist yet.

### Task 2: Extract the auth client

**Files:**
- Create: `src/services/api/auth.api.ts`
- Modify: `src/services/api/api.real.ts`

**Step 1: Move RealAuthApi**

Move the auth client into `auth.api.ts`, keeping request semantics unchanged.

**Step 2: Rewire RealApi**

Instantiate `RealAuthApi` from `api.real.ts` and remove the inlined class.

### Task 3: Verify the extraction

**Files:**
- Test: `src/services/api/__tests__/auth.api.test.ts`
- Test: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/services/api/__tests__/auth.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: PASS.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/services/api/auth.api.ts src/services/api/api.real.ts src/services/api/__tests__/auth.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
