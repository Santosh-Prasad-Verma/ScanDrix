# Review And Repository Config CLI Manual QA Checklist

## Goal

Validate the `review` and repository-config experience from a real terminal session, covering:

- local review UX and failure modes
- repository add flow
- guided setup UX
- direct setting updates
- pattern mutations with glob input
- JSON output for scripts and agents
- web handoff for advanced settings

## Preconditions

1. Build the CLI:

```bash
go build -o ./bin/scandrix ./cmd/cli/main.go
```

2. Export a valid team key:

```bash
export SCANDRIX_TEAM_KEY=your-team-key
```

3. Run commands from a real git repository with a configured remote.

4. Use the compiled entrypoint for smoke tests:

```bash
./bin/scandrix
```

## Priority 1: Review Happy Path

### 1. Review command discovery

Run:

```bash
./bin/scandrix review --help
```

Validate:

- help is readable and easy to scan
- examples are concrete and copy-pasteable
- `--staged`, `--branch`, `--interactive`, `--fix`, and `--fail-on` are easy to discover

### 2. Review current working tree

Run:

```bash
./bin/scandrix review
```

Validate:

- command explains what is being reviewed
- successful output is readable in terminal mode
- if there are no changes, the message is actionable and suggests next steps

### 3. Review staged changes

Run:

```bash
./bin/scandrix review --staged
```

Validate:

- staged scope behaves as expected
- if nothing is staged, the message explicitly suggests staging files or reviewing the full working tree

### 4. Fail review on severity threshold

Run:

```bash
./bin/scandrix review --fail-on error
```

Validate:

- command exits with code `1` only when matching issues exist
- output explains why the command failed
- the `--fail-on` threshold shown in the message matches the user input exactly

### 5. Interactive and quick-fix paths

Run:

```bash
./bin/scandrix review --interactive
./bin/scandrix review --fix
```

Validate:

- interactive mode is easy to enter when issues exist
- `--fix` clearly explains when there are no auto-fixable issues
- the next step suggested by `--fix` is useful, not generic

### 6. Invalid review option combinations

Run:

```bash
./bin/scandrix review --interactive --fix
./bin/scandrix review --interactive --prompt-only
./bin/scandrix review --fail-on nope
```

Validate:

- command fails fast before starting analysis
- errors explain the invalid combination or invalid value clearly
- hints point the user to `scandrix review --help`

## Priority 2: Repository Config Happy Path

### 7. Command discovery

Run:

```bash
./bin/scandrix config --help
./bin/scandrix config remote --help
./bin/scandrix config remote set --help
```

Validate:

- help is readable and easy to scan
- `remote` commands are discoverable from `config --help`
- wording says the CLI updates the repository's current state directly
- no mention of reset/default semantics

### 8. Add current repository without setup

Run:

```bash
./bin/scandrix config --remote . --no-prompt
```

Validate:

- repository is added successfully
- command exits cleanly
- no interactive prompt appears

### 9. Inspect repository settings

Run:

```bash
./bin/scandrix config remote show .
```

Validate:

- repository name is correct
- booleans are easy to read
- severity is visible
- pattern lists are shown clearly
- output is usable without reading docs

### 10. Run guided setup

Run:

```bash
./bin/scandrix config remote setup .
```

Validate:

- prompts are short and clear
- sections are understandable
- glob helper text helps instead of confusing
- preview matches the chosen answers
- apply step is explicit before persisting

Suggested inputs:

- automated review: `yes`
- auto approve: `no`
- minimum severity level: `critical`
- ignored files: `**/*.lock`, `dist/**`
- base branches: `main`, `release/*`
- ignored titles: `wip*`, `draft*`

### 11. Confirm persisted state

Run:

```bash
./bin/scandrix config remote show .
```

Validate:

- values match the setup choices exactly
- glob patterns are preserved as typed

## Priority 3: Direct Mutation Flow

### 12. Set scalar values directly

Run:

```bash
./bin/scandrix config remote set . review.enabled true
./bin/scandrix config remote set . review.autoApprove false
./bin/scandrix config remote set . review.requestChanges.minSeverity critical
```

Validate:

- command naming feels predictable
- success output is obvious
- follow-up `show` reflects the updated values

### 13. Add and remove patterns with human aliases

Run:

```bash
./bin/scandrix config remote add-ignore-file . "coverage/**"
./bin/scandrix config remote add-base-branch . "develop"
./bin/scandrix config remote add-ignore-title . "chore(release)*"
./bin/scandrix config remote remove-ignore-file . "coverage/**"
./bin/scandrix config remote remove-base-branch . "develop"
./bin/scandrix config remote remove-ignore-title . "chore(release)*"
```

Validate:

- aliases feel better than generic field names
- add does not duplicate existing patterns
- remove only removes the chosen pattern
- patterns remain untouched otherwise

### 14. Add and remove patterns with generic commands

Run:

```bash
./bin/scandrix config remote add-pattern . ignore-files "tmp/**"
./bin/scandrix config remote remove-pattern . ignore-files "tmp/**"
```

Validate:

- generic commands still work for scripts
- supported field names are understandable from help and error messages

## Priority 4: JSON and Agent Flow

### 15. Validate JSON output

Run:

```bash
./bin/scandrix config --remote . --json
./bin/scandrix config remote list --json
./bin/scandrix config remote show . --json
./bin/scandrix config remote set . review.enabled true --json
./bin/scandrix config remote add-ignore-file . "dist/**" --json
./bin/scandrix config remote open . --section suggestion-control --json
```

Validate:

- output is valid JSON
- no human-oriented text is mixed into stdout
- payload shape is stable enough for automation
- command result contains enough context to chain the next step

### 16. Validate structured setup preview

Run:

```bash
./bin/scandrix config remote setup . --json
```

Validate:

- payload includes current settings and next settings
- cancellation returns a non-applied result cleanly
- apply returns the saved settings

## Priority 5: Browser Handoff

### 17. Open advanced settings

Run:

```bash
./bin/scandrix config remote open . --section suggestion-control
./bin/scandrix config remote open . --section general
```

Validate:

- browser opens successfully
- CLI prints the repository and section path clearly
- unsupported sections fail with a helpful error
- behavior is still useful even without deep-link support

## Priority 6: Error Handling

### 18. Invalid key

Run:

```bash
./bin/scandrix config remote set . invalid.key true
```

Validate:

- command fails fast
- error tells the user the key is invalid
- message nudges the user toward valid usage

### 19. Invalid repository context

Run outside a git repository:

```bash
./bin/scandrix config --remote .
```

Validate:

- error is direct and actionable
- no stack trace leaks to the user

### 20. Missing authentication

Unset the team key and retry:

```bash
unset SCANDRIX_TEAM_KEY
./bin/scandrix config remote list
```

Validate:

- auth failure is clear
- message points the user to the correct auth/setup path

## Sign-off Questions

Before shipping broadly, answer these with a clear yes or no:

- Can a first-time user add and configure a repository without reading source code?
- Do the commands feel predictable between guided and direct modes?
- Are glob-based fields explained clearly enough for real users?
- Is the JSON output clean enough for scripts and agents?
- Is the web handoff good enough until deep-link URLs exist?

## Known Product Limitation

`scandrix config remote open` does not deep-link to a repository section yet. It opens the ScanDrix app and prints the navigation path. This is acceptable for smoke testing, but should be upgraded once the app route is defined.
