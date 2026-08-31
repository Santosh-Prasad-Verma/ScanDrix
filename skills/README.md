# ScanDrix Skills

This directory contains the autonomous AI agent skills shipped with the ScanDrix CLI.

## Included Skills

- `scandrix-review`
  - Run local ScanDrix security and quality code review for workspace changes using the installed CLI. Emits prompt-friendly markdown with `--prompt-only` or JSON with `--agent`.
- `scandrix-review-dev`
  - Run the local ScanDrix CLI build against a local/dev backend API for debugging or offline development.
- `scandrix-business-rules-validation`
  - Canonical skill for business rules and acceptance criteria validation (`scandrix pr business-validation`) against task trackers (Jira, Linear, GitHub Issues).
- `scandrix-pr-suggestions-resolver`
  - Fetch remote pull request review suggestions and automatically triage or apply fixes in code with judgment.
- `scandrix-centralized-config`
  - Manage organization-wide centralized security and quality configurations (`scandrix config centralized status|init|sync|disable`).
- `scandrix-rules`
  - Create, update, view, and sync ScanDrix organization rules (`scandrix rules create|update|view|sync|validate`).
- `scandrix-trace`
  - Session decision memory and architectural recall (`scandrix trace <paths>`, `pin`, `forget`, `ui`).
- `hunk-review`
  - Drive a live Hunk diff review session: inspect, navigate, reload, and leave inline comments via `hunk session ...`.

## Trigger Map (Recommended)

- **User mentions `review`, `commit`, `push`, `open PR`, `merge`, `quality gate`, `security scan`**
  - Prefer `scandrix-review` (or `scandrix-review-dev` for local backend development).
  - If the user asks to commit, push, or merge and no review has run yet, proactively offer to run `scandrix review --staged` first.
- **User mentions `business validation`, `acceptance criteria`, `task requirements`, `implementation vs task`**
  - Use `scandrix-business-rules-validation`.
- **User asks to fetch, triage, or apply PR suggestions**
  - Use `scandrix-pr-suggestions-resolver`.
- **User asks to manage centralized configuration, source repo, or sync rules across teams**
  - Use `scandrix-centralized-config`.
- **User asks to create, modify, inspect, or test custom review rules**
  - Use `scandrix-rules`.
- **User asks why code is written a certain way, or wants to check architectural decisions before modifying unfamiliar code**
  - Use `scandrix-trace`.
- **User has a Hunk or TUI diff session open, or asks to navigate/comment interactively**
  - Use `hunk-review`.

## Agent Installation & Sync

- `scandrix skills list` — list all bundled skills and descriptions.
- `scandrix skills install` — sync bundled skills into detected local agent roots (`.cursor/rules/`, `.claude/`, `.agents/skills/`).
- `scandrix skills resync` — re-synchronize all managed skills to latest versions.
- `scandrix skills uninstall` — cleanly remove managed skills.
- `scandrix skills prompt` — generate XML (`<available_skills>`) prompt payload for LLM system prompt injection.
- `scandrix skills prompt --json` — generate JSON prompt payload for tooling and MCP servers.

## For Integrators

Recommended injection pattern:
1. Run `scandrix skills prompt` during agent session bootstrap.
2. Inject the XML payload into your system/developer prompt under `Available skills`.
3. The agent activates the corresponding skill only when matching triggers occur.
