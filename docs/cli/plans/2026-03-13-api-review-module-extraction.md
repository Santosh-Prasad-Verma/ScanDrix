# API Review Module Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract the review API client from `src/services/api/api.real.ts` into a dedicated module without changing the public `RealApi.review` behavior.

**Architecture:** Create a `review.api.ts` module that encapsulates review-specific request building and auth header selection on top of the shared `requestWithRetry` helper. Keep `api.real.ts` as the assembler that wires `RealReviewApi` into `RealApi`.

**Tech Stack:** TypeScript, Vitest

### Task 1: Pin the new review API module

**Files:**
- Create: `src/services/api/__tests__/review.api.test.ts`

**Step 1: Write the failing test**

Add focused tests for `RealReviewApi` that verify:
- `analyze` uses `Authorization` and `teamId` query for bearer tokens
- `getPullRequestSuggestions` uses `X-Team-Key` for team-key auth
- `triggerBusinessValidation` serializes only provided fields

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/review.api.test.ts`

Expected: FAIL because the new module does not exist yet.

### Task 2: Extract the review client

**Files:**
- Create: `src/services/api/review.api.ts`
- Modify: `src/services/api/api.real.ts`

**Step 1: Move RealReviewApi**

Move the entire review client implementation into `review.api.ts`, keeping request semantics unchanged.

**Step 2: Rewire RealApi**

Instantiate `RealReviewApi` from `api.real.ts` and remove the inlined class.

### Task 3: Verify the extraction

**Files:**
- Test: `src/services/api/__tests__/review.api.test.ts`
- Test: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/services/api/__tests__/review.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: PASS.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/services/api/review.api.ts src/services/api/api.real.ts src/services/api/__tests__/review.api.test.ts src/services/api/__tests__/api.real.test.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
