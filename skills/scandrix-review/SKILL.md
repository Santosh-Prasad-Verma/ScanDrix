---
name: scandrix-review
description: Use when the user wants ScanDrix to review local changes, run `scandrix review` or `--prompt-only`, fix ScanDrix review findings, or check commit, push, or merge readiness.
---

# ScanDrix Review

## Goal

Use the ScanDrix CLI to review changes and resolve security, quality, and architecture issues. Prefer machine-friendly output via `--prompt-only` or structured output via `--agent`, then apply targeted fixes in code.

If the request is to validate local changes against business rules, task requirements, or acceptance criteria, use `scandrix-business-rules-validation` instead. `scandrix review` runs code security and quality policies.

## Trigger Hints

- Treat mentions of `review`, `commit`, `push`, `open PR`, `merge`, `quality gate`, `security scan`, or `ready to ship` as triggers for this skill.
- For commit/push/merge requests, proactively ask to run ScanDrix review first when a fresh review has not run yet in the current task.

## Workflow

1. **Ensure ScanDrix CLI is available**:
   - Run `scandrix --help` to confirm.
   - If missing, ask the user to install via `curl -fsSL https://get.scandrix.dev/install | sh` or build locally with `go build ./cmd/cli`.

2. **Ensure authentication if required**:
   - If `scandrix review` fails with auth, ask the human to authenticate with `scandrix auth login` in their terminal, then retry after they confirm.
   - For team keys, use `scandrix auth team-key --key <key>` when provided by the user.

3. **Run review using prompt-only output**:
   - Default staged changes: `scandrix review --staged --prompt-only`
   - Whole workspace: `scandrix review --prompt-only`
   - Specific files: `scandrix review --prompt-only <files...>`
   - Outgoing branch commits: `scandrix review --branch <name> --prompt-only`
   - Specific commit: `scandrix review --commit <sha> --prompt-only`
   - Fast deterministic mode: add `--fast`
   - Deep LLM multi-critic pass: add `--heavy`
   - Auto-apply suggested fixes directly: add `--fix`

4. **Parse results and apply fixes**:
   - Use the output to locate files, line numbers, and suggested diffs.
   - Make minimal, targeted changes to address each issue.
   - If an issue is not actionable or is a false positive, explain why and skip.

5. **Re-run review to verify**:
   - After fixes, rerun `scandrix review --staged --prompt-only` to confirm issues are resolved and clean review passes.

## Notes

- Prefer `--prompt-only` for predictable text parsing in agent contexts.
- Use `--agent` with `--fields <csv>` when structured JSON payload is needed.
- Avoid `--interactive` / `--tui` in autonomous agent loops unless the user specifically asks to launch the terminal UI.
