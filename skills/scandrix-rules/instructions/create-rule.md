---
name: create-rule
description: ScanDrix Rule Creation Guidelines - Use when the user wants to create a new ScanDrix Rule to enforce security, quality, or architectural standards.
---

# ScanDrix Rule Creation Guidelines

## Overview

When creating a new ScanDrix Rule, it is critical to ensure that the rule is clear, actionable, and aligned with code quality, security, and project architecture. A well-defined rule catches violations accurately without generating false-positive alert fatigue for developers.

## Workflow for Creating a ScanDrix Rule

1. **Collect User Intent**:
   - Understand the specific security vulnerability, coding anti-pattern, or compliance policy the user wants to guard against.
   - Clarify edge cases (e.g. should test files `*_test.go` or `*.spec.ts` be exempted via glob?).

2. **Draft the Rule Definition**:
   - Formulate a clean regex or AST match pattern.
   - Assign appropriate severity: `LOW`, `MEDIUM`, `HIGH`, or `CRITICAL`.
   - Choose scope: `file` (default) or `pull_request`.
   - Set path glob filter if the rule only applies to specific directories (e.g. `cmd/**`, `internal/api/**`).

3. **Review with User**:
   - Present the drafted title, pattern, severity, and path pattern before executing.

4. **Execute CLI Command**:
   ```bash
   scandrix rules create \
     --title "<title>" \
     --pattern "<regex-pattern>" \
     --severity <LOW|MEDIUM|HIGH|CRITICAL> \
     [--repo-id <repository-id>] \
     [--scope <file|pull_request>] \
     [--path <glob-pattern>] \
     [--category <security|quality|architecture|performance>]
   ```
   *Note: If `--repo-id` is omitted, the rule defaults to `global` (organization-wide).*

5. **Centralized Config Behavior**:
   - If centralized configuration is active, creating a rule returns a centralized PR URL (e.g. `https://github.com/org/governance-repo/pull/12`).
   - Report the PR link clearly and explain that the rule will become active once merged and synced.

## Production Examples

### 1. Secret Scanning (Critical)
```bash
scandrix rules create "Prohibit Hardcoded JWT Secrets" \
  --pattern "jwt\.sign\([^,]+,\s*['\"][a-zA-Z0-9_\-]{8,}['\"]" \
  --severity CRITICAL \
  --category security
```

### 2. SQL Injection Guard (High)
```bash
scandrix rules create "Prohibit Formatted SQL Query Concatenation" \
  --pattern "(?i)fmt\.Sprintf\(\"(SELECT|INSERT|UPDATE|DELETE).*%s" \
  --severity HIGH \
  --category security \
  --path "internal/database/**"
```

### 3. Production Logging Hygiene (Medium)
```bash
scandrix rules create "Avoid Naked fmt.Println in API Handlers" \
  --pattern "fmt\.(Print|Printf|Println)\(" \
  --severity MEDIUM \
  --category quality \
  --path "internal/api/**"
```
