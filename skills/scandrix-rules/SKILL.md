---
name: scandrix-rules
description: Use when the user wants to create, update, view, or sync ScanDrix organization rules via `scandrix rules` commands.
---

# ScanDrix Custom Rules

## Overview

ScanDrix Rules are automated security, quality, and architectural constraints evaluated across codebase changes during review, commit hooks, and CI/CD pipelines.

## Goal

Manage ScanDrix Rules through ScanDrix CLI only. Do not manually edit remote databases.

## Workflow

### 1. Identify the requested action:
- `init`: Generate local `.scandrix/rules.yaml` starter file.
- `create`: Add a new rule to the organization catalog.
- `view` / `list`: Display active rules.
- `sync`: Pull remote organization rules into `.scandrix/rules.yaml`.
- `validate` / `test`: Test local YAML syntax and regex patterns.

### 2. Commands:

```bash
# Initialize local starter rules
scandrix rules init

# Create a custom rule
scandrix rules create "Prohibit Raw SQL Queries" --pattern "(?i)(SELECT|INSERT|UPDATE|DELETE).*FROM" --severity HIGH

# List active rules
scandrix rules list

# Sync remote organization rules
scandrix rules sync

# Validate local rules syntax
scandrix rules validate
```

### 3. Rule Structure:
- **Title**: Short, actionable summary of the violation.
- **Pattern**: Regular expression or AST query to match.
- **Severity**: `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`.
- **Category**: `security`, `quality`, `architecture`, `owasp`, `performance`.
- **Remediation**: Clear instructions or diff on how to resolve.
