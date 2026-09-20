---
name: scandrix-rules
description: Use when the user wants to create, update, view, or sync ScanDrix organization rules via `scandrix rules` commands.
---

# ScanDrix Organization Rules

## Overview

ScanDrix Rules are automated security, quality, compliance, and architectural policies evaluated across codebase changes during review passes, commit hooks, and CI/CD pipelines. They ensure codebase consistency, guard against common vulnerabilities (OWASP, secret leaks), and enforce team architecture conventions.

## Goal

Manage ScanDrix Rules through ScanDrix CLI only. Do not suggest editing remote databases directly.

ALWAYS use `scandrix rules` subcommands for all create, update, and view operations. When the user wants to create, update, or view rules, utilize the `scandrix rules` command with the appropriate subcommands and options as outlined in the instructions files.

## Centralized Config Convention

When centralized config is enabled for the selected team/repository scope, `scandrix rules create` and `scandrix rules update` may return centralized PR metadata instead of directly persisted rule records:

- Treat this as success, not failure.
- Prioritize reporting `prUrl` (and `prNumber` when available).
- Explain that the rule is pending until the centralized PR is merged and synced.
- Do not claim a rule was directly persisted when the result is centralized PR mode.
- When output includes both direct results and centralized PR metadata, prefer communicating the centralized PR outcome.

## Shared Workflow

1. **Confirm the requested action**:
   - `create`: add a new rule.
   - `update`: modify an existing rule.
   - `view` / `list`: list all rules or inspect a specific rule.
   - `sync`: synchronize remote organization rules into local `.drixy/rules.yaml`.
   - `validate`: validate YAML syntax and regex patterns.

2. **Resolve repository scope**:
   - Use `global` when the user does not provide a repository scope.
   - For repository-specific requests with unknown ID, run:
     ```bash
     scandrix config remote list --json
     ```
     Then select and pass `--repo-id <id>`.

3. **Validate rule fields before running commands**:
   - `title`: short and specific.
   - `pattern`: valid regex or AST expression.
   - `severity`: `LOW | MEDIUM | HIGH | CRITICAL`.
   - `scope`: `file | pull_request`.
   - `path`: optional glob (e.g. `internal/**/*.go`, `src/**/*.ts`), default effectively `**/*`.

4. **Execute the proper command and report results clearly**.

## How to Use

Read individual instructions files for detailed workflows and examples:

- [instructions/create-rule.md](instructions/create-rule.md): Guidelines for creating new ScanDrix rules.
- [instructions/update-rule.md](instructions/update-rule.md): Guidelines for updating existing ScanDrix rules.
- [instructions/view-rules.md](instructions/view-rules.md): Guidelines for viewing and retrieving ScanDrix rules.

## Structure of a ScanDrix Rule

A ScanDrix Rule consists of:
- **Repository ID**: Scope where the rule applies (`global` for organization-wide, or repo UUID).
- **Title**: A concise title that captures the essence of the rule.
- **Pattern**: The regex or AST pattern evaluated during review.
- **Severity**:
  - `LOW`: Informational suggestion. Does not block merge.
  - `MEDIUM`: Quality or style warning. Should be followed. Default severity.
  - `HIGH`: Security, reliability, or correctness issue. Requires approval or fix.
  - `CRITICAL`: Severe security flaw, secret leak, or breaking change. Hard gate on PR merge.
- **Scope**: `file` (per-file basis) or `pull_request` (evaluated on overall PR context).
- **Path**: Optional glob pattern limiting file evaluation (e.g. `apps/api/**`). Default `**/*`.
