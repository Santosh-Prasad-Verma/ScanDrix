---
name: scandrix-trace
description: Use when about to edit files in an area you have not touched yet, or when the user asks why code is the way it is. Reads decisions already recorded for those paths via `scandrix trace <paths>`, so deliberate tradeoffs are not undone.
---

# ScanDrix Trace

## Overview

ScanDrix Trace records the reasoning behind code changes — architectural choices, conventions, tradeoffs, deliberate deferrals — and keys them to the paths they apply to. Diffs show what changed; trace explains why.

The record covers work done by anyone on the team, not just this session, so it is the fastest way to discover what teammates decided about a module.

## When to Use This

Call trace recall **before editing**, not after:
- Before your first edit in a package or directory you have not touched this session.
- When the user asks why something is implemented the way it is.
- When you are about to "refactor" something that looks unusual — it may be a deliberate recorded tradeoff.
- When choosing between two architectural approaches.

## Reading Decisions

```bash
scandrix trace internal/engine/runner.go
scandrix trace internal/cli internal/rules
scandrix trace internal/cli --limit 10
scandrix trace internal/cli --format json
```

## Correcting & Pinning Decisions

```bash
# Pin an important decision into review context
scandrix trace pin <id>

# Forget an outdated or incorrect decision
scandrix trace forget <id>

# Launch the interactive local trace cockpit
scandrix trace ui
```
