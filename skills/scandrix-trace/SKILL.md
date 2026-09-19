---
name: scandrix-trace
description: Use when about to edit files in an area you have not touched yet, or when the user asks why code is the way it is. Reads the decisions already recorded for those paths via `scandrix trace <paths>`, so deliberate tradeoffs are not undone and settled questions are not re-litigated.
---

# ScanDrix Trace

## Overview

ScanDrix Trace records the reasoning behind changes — architectural choices, conventions, tradeoffs, deliberate deferrals — and keys them to the paths they apply to. Diffs show what changed; trace explains why.

The record covers work done by anyone on the team, not just this session, so it is the fastest way to find out what a teammate decided about a module months ago.

## When to Use This

Call recall **before editing**, not after:

- Before your first edit in a directory or package you have not touched this session.
- When the user asks why something is implemented the way it is.
- When you are about to "clean up" or "refactor" something that looks unusual — it may be a recorded tradeoff.
- When you are choosing between two approaches in an area someone else owns.

Do **not** call it for a file you already recalled this session, and do not call it in a repository where `scandrix trace status` reports nothing captured.

## Reading Decisions

Reading needs no subcommand. Paths are positional on the group itself:

```bash
scandrix trace internal/billing/invoice.go
scandrix trace internal/billing internal/payments
scandrix trace internal/billing --limit 10
```

A directory returns everything scoped under it. A file returns everything whose scope covers it, including decisions recorded against its directory.

Machine-readable output, for when you want to parse or filter it programmatically:

```bash
scandrix trace internal/billing --format json
```

If a path collides with a subcommand name (`enable`, `status`, `ui`, …), disambiguate it with `--`:

```bash
scandrix trace -- status
```

Do not invent a `recall` subcommand — reading needs no verb.

## Reading the Output

Each decision carries:

- **type**: `architectural_decision`, `convention`, `tradeoff`, `implementation_detail`, `tooling`, `other`.
- **origin**: `human` if the person asked for it, `agent` if the agent chose it unprompted, `collaborative` if it was settled between them.
- **confidence**: 0 to 1, as judged during distillation.
- **scope**: the paths it applies to.
- **source**: `local` (recorded on this machine) or `remote` (came with the repository, from a teammate).
- **id**: the unique handle for `forget` and `pin`.

A `tradeoff` with high confidence is the most critical item on the list: it is usually the exact reason the code looks the way it does. Treat one as a constraint on your changes unless the user explicitly tells you otherwise, and state it out loud rather than silently working around it.

Treat `origin: human` as stronger than `origin: agent`. Low confidence is a hint, not a rule.

## Correcting the Record

When a decision is plainly wrong, obsolete, or the user asks to remove it:

```bash
scandrix trace forget <id>
```

When a decision must never be pruned or dropped from the review context pack:

```bash
scandrix trace pin <id>
```

Do not run either of these on your own judgment alone — confirm with the user first, because both are shared corrections across the team.

## Trace Cockpit

To visually inspect and query the entire architectural timeline across git history:

```bash
scandrix trace ui
```
Runs a local lightweight TUI/web interface for exploring recorded rationale and linked commit hashes.
