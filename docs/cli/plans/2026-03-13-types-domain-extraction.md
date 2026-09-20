# Types Domain Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Break `src/types/index.ts` into smaller domain-oriented files while keeping the existing `../types/index.js` imports working.

**Architecture:** Create dedicated modules such as `auth.ts`, `review.ts`, `config.ts`, `repo-config.ts`, `cli.ts`, and `errors.ts`. Keep `index.ts` as a compatibility barrel that re-exports the split modules, so the rest of the codebase can migrate gradually with minimal churn.

**Tech Stack:** TypeScript, Vitest

### Task 1: Pin the new runtime type modules

**Files:**
- Create: `src/types/__tests__/errors.test.ts`
- Create: `src/types/__tests__/index.test.ts`

**Step 1: Write the failing tests**

Add focused tests that verify:
- `errors.ts` exports `ApiError` and `AuthError`
- `index.ts` still re-exports those runtime classes

**Step 2: Run tests to verify they fail**

Run: `go test ./test/cli/... src/types/__tests__/errors.test.ts src/types/__tests__/index.test.ts`

Expected: FAIL because the new module does not exist yet.

### Task 2: Split types by domain

**Files:**
- Create: `src/types/auth.ts`
- Create: `src/types/review.ts`
- Create: `src/types/config.ts`
- Create: `src/types/repo-config.ts`
- Create: `src/types/cli.ts`
- Create: `src/types/errors.ts`
- Modify: `src/types/index.ts`

**Step 1: Move domain types**

Move each group of related interfaces and aliases into its dedicated file, preserving names.

**Step 2: Preserve barrel compatibility**

Replace the inline declarations in `index.ts` with re-exports from the new files and keep the existing re-export of `memory.ts`.

### Task 3: Verify the extraction

**Files:**
- Test: `src/types/__tests__/errors.test.ts`
- Test: `src/types/__tests__/index.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/types/__tests__/errors.test.ts src/types/__tests__/index.test.ts`

Expected: PASS.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/types/errors.ts src/types/index.ts src/types/__tests__/errors.test.ts src/types/__tests__/index.test.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
