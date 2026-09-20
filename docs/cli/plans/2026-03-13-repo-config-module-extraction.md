# Repo Config Module Extraction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract the repository-configuration slice from `src/commands/config.ts` into a dedicated module while preserving the existing CLI behavior and public exports.

**Architecture:** Move repo-config actions and remote/repo command registration into `src/features/repo-config/`, keep `src/commands/config.ts` as the top-level `config` command facade, and re-export the existing action functions from there so current tests and imports remain stable. Use focused tests to pin the new wiring before moving production code.

**Tech Stack:** TypeScript, Commander, Vitest

### Task 1: Pin the new repo-config command module

**Files:**
- Create: `src/features/repo-config/__tests__/command.test.ts`

**Step 1: Write the failing test**

Assert that a dedicated repo-config command registrar creates the `remote` subtree with the expected core subcommands (`add`, `list`, `show`, `setup`, `set`, `open`, `add-pattern`, `remove-pattern`).

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/features/repo-config/__tests__/command.test.ts`

Expected: FAIL because the new repo-config command module does not exist yet.

### Task 2: Extract repo-config actions and command registration

**Files:**
- Create: `src/features/repo-config/actions.ts`
- Create: `src/features/repo-config/command.ts`
- Modify: `src/commands/config.ts`

**Step 1: Move action orchestration**

Move `configRepoAction`, `configRepoAddAction`, `configRemoteAction`, `configRemoteAddAction`, `configRepoListAction`, `configRepoShowAction`, `configRepoSetupAction`, `configRepoOpenAction`, `configRepoSetAction`, `configRepoPatternAddAction`, and `configRepoPatternRemoveAction` into `src/features/repo-config/actions.ts`.

**Step 2: Move command wiring**

Create a repo-config command registration helper in `src/features/repo-config/command.ts` that builds the `remote` and hidden `repo` command trees.

**Step 3: Keep backward compatibility**

In `src/commands/config.ts`, keep the top-level `config` command and its `-r/--remote` shortcut, import the new helpers, and re-export the existing action functions so current tests and imports still work.

### Task 3: Verify the extraction

**Files:**
- Test: `src/features/repo-config/__tests__/command.test.ts`
- Test: `src/commands/__tests__/config.repo.test.ts`

**Step 1: Run focused tests**

Run: `go test ./test/cli/... src/features/repo-config/__tests__/command.test.ts src/commands/__tests__/config.repo.test.ts`

Expected: PASS with no behavior regressions.

**Step 2: Run lint**

Run: `./node_modules/.bin/eslint src/features/repo-config/actions.ts src/features/repo-config/command.ts src/features/repo-config/__tests__/command.test.ts src/commands/config.ts`

Expected: exit code 0.

**Step 3: Run build**

Run: `go build -o ./bin/scandrix ./cmd/cli/main.go`

Expected: exit code 0.
