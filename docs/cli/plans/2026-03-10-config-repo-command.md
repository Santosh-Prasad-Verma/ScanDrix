# Config Repo Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `scandrix config repo .` so the CLI can add the current repository to ScanDrix using team-key authentication.

**Architecture:** Introduce a new `config` command and a small repository-config service. The command stays thin, while the service resolves the local git repository, loads team-key metadata, calls the CLI config API, and returns deterministic user-facing outcomes such as success, already added, not found, and missing integration.

**Tech Stack:** Commander, existing git/auth/config utilities, `RealApi`, Vitest.

### Task 1: Add failing tests for the new API contract

**Files:**
- Modify: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Write the failing test**

- Verify `X-Team-Key` is used for `GET /cli/config/repositories/available`
- Verify `X-Team-Key` is used for `POST /cli/config/repositories`

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts`

Expected: FAIL because config API methods do not exist yet.

### Task 2: Add failing tests for repo config service behavior

**Files:**
- Create: `src/services/__tests__/repo-config.service.test.ts`

**Step 1: Write the failing test**

- adds current repo when `.` resolves to an available unselected repo
- returns already-added when current repo is already selected
- fails when no team-key config exists
- fails when saved config has no `teamId`
- fails when current directory is not a git repo or has no remote match

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/__tests__/repo-config.service.test.ts`

Expected: FAIL because service does not exist yet.

### Task 3: Add failing tests for the command action

**Files:**
- Create: `src/commands/__tests__/config.repo.test.ts`

**Step 1: Write the failing test**

- `config repo .` prints success
- prints already-added message
- exits with code 1 on invalid config / missing repo

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.test.ts`

Expected: FAIL because command does not exist yet.

### Task 4: Implement the config API, service, and command

**Files:**
- Modify: `src/services/api/api.interface.ts`
- Modify: `src/services/api/api.real.ts`
- Modify: `src/types/index.ts`
- Modify: `src/utils/config.ts`
- Create: `src/services/repo-config.service.ts`
- Create: `src/commands/config.ts`
- Modify: `src/cli.ts`

**Step 1: Write minimal implementation**

- add config API methods and types
- add repo config service with `.` resolution through git remote
- require team-key config with `teamId`
- add `config repo [repository]` command

**Step 2: Run focused tests**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts src/services/__tests__/repo-config.service.test.ts src/commands/__tests__/config.repo.test.ts`

Expected: PASS.

### Task 5: Update team-key auth persistence for future commands

**Files:**
- Modify: `src/commands/auth/team-key.ts`
- Modify: `src/commands/__tests__/auth.team-key.test.ts`

**Step 1: Add team metadata persistence**

- persist `teamId` and `organizationId` when returned by `/cli/validate-key`

**Step 2: Run focused auth tests**

Run: `go test ./test/cli/... src/commands/__tests__/auth.team-key.test.ts`

Expected: PASS.

### Task 6: Expand config repo into an explicit command group

**Files:**
- Modify: `src/services/api/api.interface.ts`
- Modify: `src/services/api/api.real.ts`
- Modify: `src/services/repo-config.service.ts`
- Modify: `src/commands/config.ts`
- Modify: `src/services/api/__tests__/api.real.test.ts`
- Modify: `src/services/__tests__/repo-config.service.test.ts`
- Modify: `src/commands/__tests__/config.repo.test.ts`

**Step 1: Write the failing test**

- `config repo list` returns selected repositories
- `config repo add .` behaves the same as `config repo .`

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts src/services/__tests__/repo-config.service.test.ts src/commands/__tests__/config.repo.test.ts`

Expected: FAIL until list API + command group are implemented.
