# Repo Config Review Follow-ups Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the repository-config review findings without regressing the current CLI onboarding flow.

**Architecture:** Keep the dedicated CLI repository-settings endpoints for team-key flows, but stop claiming bearer fallback works until the repository lookup path supports it. Expand the severity model to preserve the real web values and rename the CLI copy so it describes the setting actually being changed.

**Tech Stack:** TypeScript, Commander, Vitest, NestJS, Jest

### Task 1: Lock the intended behavior with tests

**Files:**
- Modify: `src/services/__tests__/repo-settings.service.test.ts`
- Modify: `src/commands/__tests__/config.repo.test.ts`
- Modify: `/Users/wellingtonsantana/Documents/scandrix-git/scandrix-ai/libs/code-review/application/use-cases/configuration/__tests__/get-cli-repository-settings-use-case.spec.ts`
- Modify: `/Users/wellingtonsantana/Documents/scandrix-git/scandrix-ai/libs/code-review/application/use-cases/configuration/__tests__/update-cli-repository-settings-use-case.spec.ts`

**Step 1: Write the failing tests**
- Add a CLI service test proving bearer auth does not use the `/cli/config/repositories/*` path for repository resolution.
- Add tests proving `medium` is preserved on read/update.
- Add a command/help test proving the copy no longer says "request changes severity" or promises bearer fallback.

**Step 2: Run tests to verify they fail**
- Run the smallest focused test commands for each file and confirm failure is for the expected missing behavior.

### Task 2: Fix auth and severity semantics

**Files:**
- Modify: `src/services/repo-settings.service.ts`
- Modify: `src/types/index.ts`
- Modify: `src/utils/repo-settings-schema.ts`
- Modify: `src/services/repo-settings-wizard.service.ts`
- Modify: `src/formatters/repo-config.ts`
- Modify: `/Users/wellingtonsantana/Documents/scandrix-git/scandrix-ai/libs/code-review/application/use-cases/configuration/cli-repository-settings.types.ts`
- Modify: `/Users/wellingtonsantana/Documents/scandrix-git/scandrix-ai/libs/code-review/application/use-cases/configuration/get-cli-repository-settings.use-case.ts`
- Modify: `/Users/wellingtonsantana/Documents/scandrix-git/scandrix-ai/libs/code-review/application/use-cases/configuration/update-cli-repository-settings.use-case.ts`

**Step 1: Implement the minimal production changes**
- Remove or gate the unsupported bearer fallback path so the CLI is honest about auth requirements.
- Expand the repository severity type to preserve `low`, `medium`, `high`, and `critical`.
- Rename the user-facing field/copy from request-changes-specific language to minimum review severity language.

**Step 2: Re-run the focused tests**
- Run the same focused test set and verify green before touching docs.

### Task 3: Align docs and help text

**Files:**
- Modify: `src/commands/config.ts`
- Modify: `README.md`

**Step 1: Update help and README**
- Remove claims that bearer fallback works for repository settings if it still does not.
- Align examples and descriptions with the new severity wording.

**Step 2: Verify docs-adjacent tests**
- Run the config command tests again to confirm the help output still matches.

### Task 4: Final verification

**Files:**
- No code changes expected

**Step 1: Run focused verification**
- CLI: formatter, schema, settings service, api, and config command tests.
- Backend: new CLI settings use-case/controller tests.

**Step 2: Run build/lint**
- CLI: `eslint` on touched files plus `go build -o ./bin/scandrix ./cmd/cli/main.go`.
- Backend: run the available targeted tests and `go build -o ./bin/scandrix ./cmd/cli/main.go:api`; note any environment lint blocker explicitly if it remains.
