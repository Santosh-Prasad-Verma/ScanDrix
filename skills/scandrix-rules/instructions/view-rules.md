---
name: view-rules
description: ScanDrix Rule Viewing Guidelines - Use when the user wants to view or inspect existing ScanDrix Rules that govern code review and security audits.
---

# ScanDrix Rule Viewing Guidelines

## Overview

When viewing existing ScanDrix Rules, it is important to understand the full metadata of each rule: UUID, title, pattern, severity, category, scope, and path filters. This visibility ensures that developers and security engineers understand exactly what quality gates apply to their pull requests.

## Workflow for Viewing ScanDrix Rules

1. **Identify Inspection Scope**:
   - Organization-wide (global rules): default.
   - Specific repository: pass `--repo-id <uuid>` or `--repo <owner/name>`.
   - Specific rule by UUID: pass `--uuid <uuid>`.
   - Specific rule by Title: pass `--title "<name>"`.

2. **Execute CLI Command**:
   ```bash
   # List all active rules (human-readable table)
   scandrix rules list

   # List rules as structured JSON for automation/tooling
   scandrix rules list --json

   # Inspect a single rule by UUID
   scandrix rules view --uuid <uuid>

   # Filter rules for a specific repository
   scandrix rules list --repo-id <repository-uuid>
   ```

3. **Interpreting Rule Details**:
   Each rule entry includes:
   - `UUID`: Globally unique identifier for CLI updates/deletions.
   - `Repository ID`: Scope (`global` or repo-specific).
   - `Title`: Short descriptive name.
   - `Severity`: `LOW` | `MEDIUM` | `HIGH` | `CRITICAL`.
   - `Category`: `security`, `quality`, `architecture`, `owasp`, or `performance`.
   - `Path`: Glob expression limiting file matches.
   - `Status`: Active or pending centralized PR merge.

4. **Centralized Config Context**:
   When centralized governance is active, the CLI outputs rule sync status (`synced`, `pending_add`, `pending_edit`, `pending_delete`) and links to the upstream PR if changes are awaiting review.
