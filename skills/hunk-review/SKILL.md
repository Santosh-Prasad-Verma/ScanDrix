---
name: hunk-review
description: Interacts with live Hunk diff review sessions via CLI. Inspects review focus, navigates files and hunks, reloads session contents, and adds inline review comments. Use when the user has a Hunk session running or wants to review diffs interactively.
---

# Hunk Review

Hunk is an interactive terminal diff viewer. The TUI is for the user -- do NOT run `hunk diff`, `hunk show`, or other interactive commands directly in unattended agent mode. Use `hunk session *` CLI commands to inspect and control live sessions through the local daemon.

If no session exists, ask the user to launch Hunk or `scandrix tui` in their terminal first.

## Workflow

```text
1. hunk session list                                    # find live sessions
2. hunk session get --repo .                            # inspect path / repo / source
3. hunk session review --repo . --json                  # inspect file/hunk structure first
4. hunk session review --repo . --include-patch --json  # opt into raw diff text only when needed
5. hunk session context --repo .                        # check current focus when needed
6. hunk session navigate ...                            # move to the right place
7. hunk session reload -- <command>                     # swap contents if needed
8. hunk session comment add ...                         # leave one review note
9. hunk session comment apply ...                       # apply many agent notes in one stdin batch
```

## Session Selection

Most session commands accept:
- `--repo <path>` — match the live session by its current loaded repo root (most common)
- `<session-id>` — match by exact ID (use when multiple sessions share a repo)
- If only one session exists, it auto-resolves

## Commands

### Inspect
```bash
hunk session list [--json]
hunk session get (--repo . | <id>) [--json]
hunk session context (--repo . | <id>) [--json]
hunk session review (--repo . | <id>) [--json] [--include-patch]
```

### Navigate
```bash
hunk session navigate --repo . --file internal/engine/runner.go --hunk 2
hunk session navigate --repo . --file internal/engine/runner.go --new-line 120
hunk session navigate --repo . --next-comment
hunk session navigate --repo . --prev-comment
```

### Comments
```bash
hunk session comment add --repo . --file README.md --new-line 50 --summary "Verify security baseline" --author "agent"
```
