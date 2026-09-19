---
name: scandrix-centralized-config
description: Use when the user wants to manage centralized configuration via `scandrix config centralized` commands (status, init, sync, disable, and download).
---

# ScanDrix Centralized Configuration

## Overview

Centralized configuration allows engineering organizations to manage their ScanDrix review policies, security rules, and architectural standards in a single git repository, providing a unified single source of truth across all repositories and teams.

The configuration files (rules, severities, file exclusions) are versioned as code. Changes are proposed via pull request, allowing security leads to review rule modifications before they take effect across the fleet.

When centralized config is active:
- Merging a PR to the default branch of the governance repository triggers an automatic webhook sync in ScanDrix.
- Teams maintain transparent version control over their quality gates.
- Manual sync can be invoked on demand if needed.

## Goal

Manage centralized configuration through ScanDrix CLI commands only.

Use this skill when the request involves enabling centralized config, selecting the source repository, syncing configuration, disabling centralized config, or downloading generated config artifacts.

## Trigger Hints

- Mentions of centralized config, organization rules repo, config sync source repo, or single source of truth.
- Requests to run: `scandrix config centralized status|init|sync|disable|download`.
- Requests to enable, disable, or audit centralized config from the terminal.

## Workflow

1. **Confirm Authentication**:
   - Centralized configuration commands require team-key or admin token authentication:
     ```bash
     scandrix auth team-key --key <your-team-key>
     ```

2. **Check Current Centralized Status**:
   - Inspect status before making modifications:
     ```bash
     scandrix config centralized status
     ```

3. **Initialize Centralized Configuration**:
   - ALWAYS list accessible repositories first to confirm the exact `owner/repo` identifier:
     ```bash
     scandrix config remote list
     ```
   - Initialize centralized config with chosen sync mode:
     ```bash
     scandrix config centralized init <owner/repo> --sync-option <pr|manual>
     ```
   - **Behavior of `--sync-option`**:
     - `pr` (default): Generates starter rules and opens a pull request against `<owner/repo>`. The PR URL is emitted in the output. Merging the PR activates the configuration.
     - `manual`: Enables centralized tracking without opening an automatic PR. The user commits config files directly or runs a manual sync.

4. **Synchronize Centralized Config on Demand**:
   - Pulls the latest versioned rules from the governance repo and reconciles the ScanDrix database:
     ```bash
     scandrix config centralized sync
     ```
   > [!WARNING]
   > Running `sync` overrides local database rules with the git repository state. Recommend creating a backup with `download` before running manual sync.

5. **Download Centralized Config Archive**:
   - Exports the current rules and settings to a zip bundle:
     ```bash
     scandrix config centralized download --out ./scandrix-config-backup.zip
     ```

6. **Disable Centralized Configuration**:
   - Decouples the workspace from the central governance repository:
     ```bash
     scandrix config centralized disable
     ```

## Output Guidance

- Prefer passing `--json` when invoking in automated agent scripts for reliable parsing.
- Always report the PR URL if an action results in an upstream governance pull request.
