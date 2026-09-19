# Repository Config CLI Onboarding Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a repository-config onboarding flow that lets users add a repo, inspect its current ScanDrix settings, and complete the highest-value setup from the terminal without recreating the full web dashboard.

**Architecture:** Extend the existing `config` command group into a small repository-settings surface with two UX modes: a guided quick-setup wizard for humans and explicit commands for scripts and agents. Keep the command layer thin, move API and normalization logic into services, and present a clear preview before persisting any changes. Advanced settings stay in the web app, with an optional browser handoff from the CLI.

**Tech Stack:** Commander, `@inquirer/prompts`, `chalk`, existing API client/services, Vitest, `open`.

### Task 1: Freeze the first-slice settings model

**Files:**
- Modify: `src/types/index.ts`
- Modify: `src/services/api/api.interface.ts`
- Modify: `src/services/api/api.real.ts`
- Test: `src/services/api/__tests__/api.real.test.ts`

**Step 1: Write the failing test**

- cover a "get repository settings" API shape for the quick-setup fields
- cover an "update repository settings" API shape for the same fields
- cover any enum values needed for severity and list-based pattern fields

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts`

Expected: FAIL because repository settings endpoints and types do not exist yet.

**Step 3: Write minimal implementation**

- add request/response types for:
  - automated review enabled
  - auto approve enabled
  - request changes minimum severity
  - ignored file patterns
  - base branch patterns
  - ignored title patterns
- implement API client methods to fetch and update these settings for one repository

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/services/api/__tests__/api.real.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/types/index.ts src/services/api/api.interface.ts src/services/api/api.real.ts src/services/api/__tests__/api.real.test.ts
git commit -m "feat: add repository settings api contract"
```

### Task 2: Add a repository settings service with normalization

**Files:**
- Create: `src/services/repo-settings.service.ts`
- Create: `src/services/__tests__/repo-settings.service.test.ts`
- Modify: `src/services/repo-config.service.ts`

**Step 1: Write the failing test**

- resolves `.` to the current repository and fetches its settings
- returns a normalized summary object for terminal display
- accepts `owner/repo` explicitly
- rejects invalid repository targets
- preserves glob patterns as provided by the user

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings.service.test.ts`

Expected: FAIL because the service does not exist yet.

**Step 3: Write minimal implementation**

- create a service that:
  - resolves the repository reference
  - fetches repository settings from the API
  - maps raw API data into a CLI-friendly summary/update model
  - validates list inputs as non-empty patterns, without trying to rewrite user globs

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings.service.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/services/repo-settings.service.ts src/services/__tests__/repo-settings.service.test.ts src/services/repo-config.service.ts
git commit -m "feat: add repository settings service"
```

### Task 3: Add a read-only `show` command before any wizard work

**Files:**
- Modify: `src/commands/config.ts`
- Create: `src/commands/__tests__/config.repo.show.test.ts`

**Step 1: Write the failing test**

- `scandrix config repo show -r .` prints a readable settings summary
- the summary includes enabled/disabled states, severity, and pattern counts
- `--json` prints a stable machine-readable payload
- missing settings produce helpful defaults instead of blank output

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.show.test.ts`

Expected: FAIL because the `show` command does not exist yet.

**Step 3: Write minimal implementation**

- add `config repo show [repository]`
- support `--json`
- print a compact human summary by default
- surface whether values come from explicit settings or defaults if the API supports that distinction

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.show.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/commands/config.ts src/commands/__tests__/config.repo.show.test.ts
git commit -m "feat: add repository config show command"
```

### Task 4: Add the quick-setup wizard for the high-value settings only

**Files:**
- Create: `src/services/repo-settings-wizard.service.ts`
- Create: `src/services/__tests__/repo-settings-wizard.service.test.ts`
- Modify: `src/commands/config.ts`
- Create: `src/commands/__tests__/config.repo.setup.test.ts`

**Step 1: Write the failing test**

- `config repo setup -r .` prompts for:
  - automated review
  - auto approve
  - request changes minimum severity
  - ignored file patterns
  - base branch patterns
  - ignored title patterns
- wizard skips browser-only settings
- wizard returns a preview object before apply
- cancellation exits cleanly without changes

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/__tests__/config.repo.setup.test.ts`

Expected: FAIL because the wizard does not exist yet.

**Step 3: Write minimal implementation**

- use `@inquirer/prompts` for:
  - `confirm` on booleans
  - `select` on severity
  - repeated `input` loops for glob patterns
- include concise helper text and examples for every glob field
- print a preview block before apply
- add `--yes` support to accept defaults for automation later if needed

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/__tests__/config.repo.setup.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/services/repo-settings-wizard.service.ts src/services/__tests__/repo-settings-wizard.service.test.ts src/commands/config.ts src/commands/__tests__/config.repo.setup.test.ts
git commit -m "feat: add repository quick setup wizard"
```

### Task 5: Hook quick setup into `config add -r` as an optional next step

**Files:**
- Modify: `src/commands/config.ts`
- Modify: `src/commands/__tests__/config.repo.test.ts`

**Step 1: Write the failing test**

- after `config add -r .` succeeds, the CLI offers `Configure now?`
- declining preserves the current behavior and exits successfully
- accepting enters the same `repo setup` flow
- non-interactive runs do not prompt unexpectedly

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.test.ts src/commands/__tests__/config.repo.setup.test.ts`

Expected: FAIL because add-flow onboarding does not exist yet.

**Step 3: Write minimal implementation**

- add an optional post-add prompt
- ensure prompt is skipped when:
  - stdout is not interactive
  - a future `--no-prompt` flag is used
  - the command is already running in explicit non-interactive mode

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.test.ts src/commands/__tests__/config.repo.setup.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/commands/config.ts src/commands/__tests__/config.repo.test.ts src/commands/__tests__/config.repo.setup.test.ts
git commit -m "feat: offer quick setup after repository add"
```

### Task 6: Add atomic edit commands for scripts and agents

**Files:**
- Modify: `src/commands/config.ts`
- Modify: `src/services/repo-settings.service.ts`
- Create: `src/commands/__tests__/config.repo.set.test.ts`

**Step 1: Write the failing test**

- `config repo set -r . review.enabled=true`
- `config repo set -r . review.autoApprove=false`
- `config repo set -r . review.requestChanges.minSeverity=critical`
- `config repo set -r . patterns.ignoreFiles=dist/**,**/*.lock`
- invalid keys fail with a helpful error and list supported keys

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.set.test.ts`

Expected: FAIL because the `set` command does not exist yet.

**Step 3: Write minimal implementation**

- implement a constrained set of editable keys for the phase-1 settings
- accept comma-separated pattern lists for scripting
- reuse the same validation and preview formatting used by the wizard

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.set.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/commands/config.ts src/services/repo-settings.service.ts src/commands/__tests__/config.repo.set.test.ts
git commit -m "feat: add atomic repository setting updates"
```

### Task 7: Improve terminal UX with color, guidance, and preview formatting

**Files:**
- Create: `src/formatters/repo-config.ts`
- Create: `src/formatters/__tests__/repo-config.test.ts`
- Modify: `src/commands/config.ts`
- Modify: `src/services/repo-settings-wizard.service.ts`

**Step 1: Write the failing test**

- sections render with stable labels
- enabled/disabled states are colorized consistently
- previews show old vs new values when editing existing settings
- glob helper copy uses "pattern" language, not "keyword" language

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/formatters/__tests__/repo-config.test.ts`

Expected: FAIL because formatter and copy do not exist yet.

**Step 3: Write minimal implementation**

- centralize display strings and colors in one formatter
- use:
  - bold section titles
  - green/red for enabled and disabled
  - dim instructional copy
  - explicit examples for glob fields
- avoid heavy box drawing unless it improves scan speed

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/formatters/__tests__/repo-config.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/formatters/repo-config.ts src/formatters/__tests__/repo-config.test.ts src/commands/config.ts src/services/repo-settings-wizard.service.ts
git commit -m "feat: improve repository config terminal ux"
```

### Task 8: Add browser handoff and shell completion last

**Files:**
- Modify: `src/commands/config.ts`
- Modify: `README.md`
- Create: `src/commands/__tests__/config.repo.open.test.ts`

**Step 1: Write the failing test**

- `config repo open -r . --section general` opens the web dashboard for advanced editing
- help text explains when to use CLI setup versus the web
- if shell completion support is already configured in the CLI bootstrap, expose the new subcommands there too

**Step 2: Run test to verify it fails**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.open.test.ts`

Expected: FAIL because the browser handoff does not exist yet.

**Step 3: Write minimal implementation**

- add `repo open` as the explicit bridge to advanced settings
- document the phase-1 supported settings in `README.md`
- only add shell completion wiring if Commander support already exists cleanly in the CLI entrypoint; otherwise document it as a follow-up instead of forcing scope

**Step 4: Run test to verify it passes**

Run: `go test ./test/cli/... src/commands/__tests__/config.repo.open.test.ts`

Expected: PASS.

**Step 5: Commit**

```bash
git add src/commands/config.ts src/commands/__tests__/config.repo.open.test.ts README.md
git commit -m "feat: add repository config browser handoff"
```
