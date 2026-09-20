# Repo Config Wizard UX Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make repository onboarding feel guided and easier to use by improving prompt copy, switching key decisions to explicit selections, and simplifying ignored-file setup.

**Architecture:** Keep the command surface unchanged and localize the UX redesign inside the wizard service plus minimal command orchestration updates for final review actions. Preserve the existing `RepositorySettings` data contract so API and formatter layers stay stable.

**Tech Stack:** TypeScript, Commander, `@inquirer/prompts`, Vitest

### Task 1: Lock down the new wizard behavior with tests

**Files:**
- Modify: `src/services/__tests__/repo-settings-wizard.service.test.ts`
- Modify: `src/commands/__tests__/config.repo.test.ts`

**Step 1: Write failing wizard tests**

Add tests for:
- select-based general settings prompts with explanatory copy
- ignored files guided mode using recommended defaults
- final review flow that can jump back to edit `General` or `Patterns`

**Step 2: Run focused tests to verify they fail**

Run:
```bash
go test ./test/cli/... src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/__tests__/config.repo.test.ts
```

Expected: failures showing missing prompt behavior and updated setup flow expectations.

### Task 2: Implement the redesigned wizard

**Files:**
- Modify: `src/services/repo-settings-wizard.service.ts`
- Modify: `src/commands/config.ts`

**Step 1: Implement the minimal prompt redesign**

Add:
- select prompts for `Automated code review` and `Pull request approval`
- richer descriptions for general settings
- guided ignored-file flow with recommended defaults, common patterns, custom input, or skip
- review loop actions: apply, edit general, edit patterns, cancel

**Step 2: Keep the payload stable**

Ensure the wizard still returns:
- `reviewEnabled`
- `autoApproveEnabled`
- `requestChangesMinSeverity`
- `ignoredFilePatterns`
- `baseBranchPatterns`
- `ignoredTitlePatterns`

### Task 3: Verify and polish

**Files:**
- Modify: `README.md` only if CLI behavior text changed materially

**Step 1: Run focused tests**

Run:
```bash
go test ./test/cli/... src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/__tests__/config.repo.test.ts
```

Expected: pass.

**Step 2: Run targeted lint and build**

Run:
```bash
./node_modules/.bin/eslint src/services/repo-settings-wizard.service.ts src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/config.ts src/commands/__tests__/config.repo.test.ts
go build -o ./bin/scandrix ./cmd/cli/main.go
```

Expected: no lint errors; successful build.
