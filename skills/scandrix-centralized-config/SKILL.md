---
name: scandrix-centralized-config
description: Use when the user wants to manage centralized configuration via `scandrix config centralized` commands (status, init, sync, disable, and download).
---

# ScanDrix Centralized Configuration

## Overview

Centralized configuration allows engineering organizations to manage their ScanDrix rules, compliance policies, and risk thresholds in a single repository, providing a single source of truth across all workspaces.

## Goal

Manage centralized configuration through ScanDrix CLI commands only.

## When to Use

- Mentions of centralized config, organization rules repo, compliance policies, or single source of truth.
- Requests to run: `scandrix config centralized status|sync|disable`.
- Requests to add or list remote tracked repositories via `scandrix config remote add|list`.

## Workflow

1. **Verify authentication**:
   - Ensure team API key or token is configured (`scandrix auth team-key --key <key>`).

2. **Check current centralized status**:

```bash
scandrix config centralized status
```

3. **Synchronize organization rules**:

```bash
scandrix config centralized sync
```

4. **Track a remote repository in ScanDrix**:

```bash
scandrix config remote add owner/repository-name
scandrix config remote list
```

5. **Disable centralized config if migrating to standalone rules**:

```bash
scandrix config centralized disable
```
