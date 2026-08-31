---
name: scandrix-pr-suggestions-resolver
description: Use when the user wants to fetch, triage, or implement ScanDrix suggestions for an existing remote pull request via `scandrix pr suggestions`, `--pr-url`, or `--pr-number`.
---

# ScanDrix PR Suggestions Resolver

## Overview

Fetch PR suggestions via ScanDrix CLI, triage each suggestion against the PR goal, apply safe fixes, validate with build/tests, and report results.

## Workflow

### 1) Collect the PR Target
- If the user did not provide a target, ask for one of:
  - `--pr-url <url>` (e.g. `https://github.com/owner/repo/pull/123`)
  - `--pr-number <number>`
- If multiple are provided, prefer `--pr-url`.

### 2) Run ScanDrix Suggestions

```bash
scandrix pr suggestions --pr-url <url>
```

Or when running by PR number:

```bash
scandrix pr suggestions --pr-number <number>
```

### 3) Analyze Suggestions with PR Intent in Mind
- Confirm the PR objective from the conversation or commit history.
- For each suggestion:
  - Verify it aligns with the PR objective.
  - Prioritize security fixes (SQL injection, XSS, exposed tokens) and high-severity violations.
  - Skip suggestions that are out of scope or false positives, noting why.

### 4) Apply Fixes Incrementally
- Make changes per accepted suggestion.
- Keep edits minimal, clean, and idiomatic.

### 5) Validate with Tests
- Run `go test ./...` or the appropriate test suite for the modified files.

### 6) Report Results
- List suggestions applied (with line numbers and rationale).
- List suggestions skipped (with explanations).
- Confirm test results and next steps.
