---
name: update-rule
description: ScanDrix Rule Update Guidelines - Use when the user wants to update an existing ScanDrix Rule to modify its behavior, scope, severity, or pattern.
---

# ScanDrix Rule Update Guidelines

## Overview

When updating an existing ScanDrix Rule, it is important to ensure that modifications are justified, backward-compatible where necessary, and aligned with current engineering priorities. Updating an existing rule refines its detection accuracy and eliminates false positives without losing compliance history.

## Workflow for Updating a ScanDrix Rule

1. **Identify Rule UUID**:
   - If the rule UUID is unknown, search by title or list active rules:
     ```bash
     scandrix rules list --json
     ```
   - All rule updates require the `--uuid <uuid>` argument.

2. **Determine Target Updates**:
   - Only pass the fields being updated (title, pattern, severity, path glob, scope).
   - Verify that any updated regex pattern compiles and matches intended code constructs.

3. **Execute CLI Command**:
   ```bash
   scandrix rules update \
     --uuid <uuid> \
     [--title "<new-title>"] \
     [--pattern "<new-pattern>"] \
     [--severity <LOW|MEDIUM|HIGH|CRITICAL>] \
     [--path "<glob-pattern>"] \
     [--scope <file|pull_request>] \
     [--repo-id <repository-id>]
   ```

4. **Centralized Config PR Handling**:
   - When centralized config is enabled, updating a rule generates a pull request against the central governance repository.
   - Report the PR URL to the user and note that the rule update remains pending until merged.

5. **Verify and Test**:
   - Test the updated rule locally against sample code or run:
     ```bash
     scandrix rules validate
     ```
