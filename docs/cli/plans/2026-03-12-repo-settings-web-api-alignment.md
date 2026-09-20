# Repository Settings Web/API Alignment Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `scandrix config remote show/setup/set` use the same repository settings flow as the web app, instead of the temporary `/cli/config/repositories/:id/settings` endpoints.

**Architecture:** Keep repository selection on the existing `/cli/config/repositories/*` endpoints because they work with CLI team keys. Move repository settings read/write to the web-backed `/team` and `/parameters/*` endpoints behind bearer auth. Add a mapping layer between the CLI's simplified repository settings model and the web code review config payload.

**Tech Stack:** TypeScript, Commander, Vitest, fetch-based API client, existing CLI auth/config utilities

### Task 1: Add API tests for team discovery and parameters-backed repository settings

**Files:**
- Modify: `src/services/api/__tests__/api.real.test.ts`
- Reference: `src/services/api/api.real.ts`

**Step 1: Write the failing tests**

Add tests for:
- listing teams with bearer auth via `GET /team`
- reading repository settings via `GET /parameters/find-by-key?key=CODE_REVIEW_CONFIG&teamId=...`
- updating repository settings via `POST /parameters/create-or-update-code-review`
- rejecting repository settings reads when using a team key

**Step 2: Run tests to verify they fail**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts`

Expected: FAIL because `api.real.ts` does not yet expose the new requests.

**Step 3: Implement the minimal API client**

Add new `config` API methods for:
- resolving teams
- reading raw code review config by team
- saving repository code review config by team/repository

**Step 4: Run tests to verify they pass**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts`

Expected: PASS

### Task 2: Add service-level tests for bearer-based repository settings resolution

**Files:**
- Modify: `src/services/__tests__/repo-settings.service.test.ts`
- Reference: `src/services/repo-settings.service.ts`

**Step 1: Write the failing tests**

Add tests for:
- resolving the active team ID when a bearer token is available
- mapping web config into CLI settings for `show`
- merging CLI updates back into the web config payload for `set/setup`
- surfacing a clear error when only a team key is available

**Step 2: Run tests to verify they fail**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings.service.test.ts`

Expected: FAIL because the service still requires team-key auth and the old settings endpoints.

**Step 3: Implement the minimal service changes**

Update `repo-settings.service.ts` to:
- use `authService.getValidToken()`
- resolve bearer vs team-key mode
- fetch team list when bearer auth is used
- fetch/update `CODE_REVIEW_CONFIG` and map repository-scoped config fields

**Step 4: Run tests to verify they pass**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings.service.test.ts`

Expected: PASS

### Task 3: Update command messaging and docs

**Files:**
- Modify: `src/commands/__tests__/config.repo.test.ts`
- Modify: `src/commands/config.ts`
- Modify: `README.md`

**Step 1: Write the failing tests**

Add or update tests so `config remote show/setup/set` explain that bearer login is required when repository settings are accessed without a web-compatible session.

**Step 2: Run tests to verify they fail**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.test.ts`

Expected: FAIL because the command help/error copy still assumes team-key auth.

**Step 3: Implement the minimal copy/docs change**

Update help text, README and any surfaced errors to reflect:
- `add/list` can use team key
- `show/setup/set` use web settings flow and require account login if team-key-only auth is present

**Step 4: Run tests to verify they pass**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.test.ts`

Expected: PASS

### Task 4: Final verification

**Files:**
- Verify touched files only

**Step 1: Run focused automated checks**

Run:
- `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts src/services/__tests__/repo-settings.service.test.ts src/commands/__tests__/config.repo.test.ts`
- `./node_modules/.bin/eslint src/services/api/api.real.ts src/services/__tests__/repo-settings.service.test.ts src/services/repo-settings.service.ts src/commands/config.ts src/commands/__tests__/config.repo.test.ts README.md`
- `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: All commands succeed.

**Step 2: Manual smoke tests**

Run with bearer auth on an environment exposing `/parameters/*`:
- `scandrix config remote show owner/repo`
- `scandrix config remote setup owner/repo`
- `scandrix config remote set owner/repo review.enabled true`

Expected: Repository settings read/write works without `/cli/config/repositories/:id/settings`.
