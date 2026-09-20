# 🛡️ ScanDrix CLI Documentation & Developer Manual

The **ScanDrix CLI** (`scandrix`) is an enterprise-grade, autonomous AI code review and security assurance gate built in pure Go 1.24. It brings multi-agent adversarial code analysis, AST vulnerability detection, dynamic penetration testing, and coding session telemetry directly to developer terminals and CI/CD pipelines.

---

## ⚡ Quick Start

### 1. Build or Install
```bash
# Build binary directly from workspace
go build -o ./bin/scandrix ./cmd/cli/main.go

# Verify installation
./bin/scandrix version
```

### 2. Authenticate
```bash
# Interactive browser device flow or OAuth sign-in
scandrix auth login

# Or authenticate using a team API key
scandrix auth team-key --key scandrix_live_...
```

### 3. Review Code Changes
```bash
# Review current working tree diff
scandrix review

# Review only staged changes before commit
scandrix review --staged

# Review a specific branch or commit
scandrix review --branch main
scandrix review --commit HEAD~1

# Auto-apply actionable AST code fixes directly
scandrix review --fix
```

---

## 🧭 Architecture & Design Principles

ScanDrix CLI is engineered as a enterprise-grade, high-performance static analysis and AI review engine, compiled as a native, single-binary Go executable with zero runtime dependencies.

- **Fast & Lightweight**: Zero Node.js or `node_modules` required; executes local AST and regex rule evaluations in **<5ms**.
- **Deterministic Agent Envelopes**: Integrates with LLMs, Cursor, Claude Code, and Codex via `--agent` schema v1.0 standard envelopes.
- **Fail-Closed Security Gating**: Exit code 1 when vulnerabilities exceed `--fail-on-severity` threshold (`CRITICAL`, `HIGH`, `MEDIUM`, `LOW`).
- **Offline & Air-Gapped Resilient**: `--offline` flag executes pure local AST rule parsing against `.scandrix/rules.yaml` without network egress.

---

## 📖 Command Reference

### Core Commands

| Command | Description | Common Flags |
| :--- | :--- | :--- |
| `scandrix review` | Analyze local git diffs, patches, or specific files | `--staged`, `--branch`, `--fix`, `--offline`, `--fail-on` |
| `scandrix dry-run` | Preview review rules locally without blocking or updating cloud state | Same as `review` (enforces `exitCode = 0`) |
| `scandrix auth` | Manage authentication, tokens, team keys, and active session | `login`, `logout`, `status`, `token`, `team-key`, `team-status` |
| `scandrix config` | Manage remote repos, centralized org rules, and local config | `show`, `remote add/list/show`, `centralized init/sync/status` |
| `scandrix rules` | Manage, validate, and synthesize custom security & quality rules | `validate`, `view`, `create`, `update`, `generate`, `init` |
| `scandrix skills` | Synchronize ScanDrix review capabilities into AI coding assistants | `list`, `install`, `sync`, `uninstall` |
| `scandrix hook` | Install or inspect automated Git review guards | `install`, `status`, `uninstall` |
| `scandrix decisions` | Session event tracking and architectural decision memory recall | `enable`, `disable`, `capture`, `hooks` |
| `scandrix pr` | PR suggestion triage and task business rule validation | `suggestions`, `business-validation` |
| `scandrix schema` | Dynamic AST schema reflection for autonomous agent tooling | `--command <path>`, `--agent` |
| `scandrix status` | Consolidated cockpit status of workspace, git, auth, and hooks | `--agent` |

### Native Superpower Commands

| Superpower Command | Description |
| :--- | :--- |
| `scandrix chat` | Interactive autonomous AI terminal chat loop with live tool execution |
| `scandrix tui` | Rich Bubbletea interactive cockpit with live diffs and inline fix triage |
| `scandrix scan` | Multi-language AST security audits and semantic vulnerability scanning |
| `scandrix diff` | Semantic diff inspector across working tree, branches, and commits |
| `scandrix fix` | AST-guided automated remediations applied directly to source code |
| `scandrix export` | Generate compliance audit reports (`sarif`, `json`, `csv`, `markdown`) |
| `scandrix pentest` | Dynamic exploit verification and autonomous security agent testing |
| `scandrix mcp` | Standard Model Context Protocol (MCP) server over stdio for IDEs |
| `scandrix server` | Lightweight local HTTP server for API mocking and webhook testing |
| `scandrix trace` | Decision memory manager (`recall`, `pin`, `forget`, `ui`) |

---

## 🤖 AI Agent Integration & Schema Envelope v1.0

When running in automated environments or inside AI agents (Claude Code, Cursor, Codex), pass `--agent`:

```bash
scandrix review --offline --rules-only --agent --file patch.diff
```

All commands emit structured **Schema v1.0 envelopes**:

### Success Envelope
```json
{
  "ok": true,
  "command": "review",
  "data": {
    "status": "passed",
    "files_analyzed": 1,
    "total_findings": 0,
    "findings": []
  },
  "error": null,
  "meta": {
    "schemaVersion": "1.0",
    "cliVersion": "v1.2.0",
    "mode": "agent",
    "durationMs": 3
  }
}
```

### Error Envelope
```json
{
  "ok": false,
  "command": "rules view",
  "data": null,
  "error": {
    "code": "AUTH_REQUIRED",
    "message": "API error (status 401): unauthorized: missing authorization header"
  },
  "meta": {
    "schemaVersion": "1.0",
    "cliVersion": "v1.2.0",
    "mode": "agent",
    "durationMs": 1
  }
}
```

### Field Masking (`--fields`)
To reduce token usage in LLM context windows, project only the needed fields:
```bash
scandrix review --format json --fields "files_reviewed,findings.file_path,findings.severity"
```

---

## ⚙️ Configuration & Environment Hierarchy

Configuration is resolved hierarchically in order of precedence:
1. **CLI Command-Line Flags** (`--server`, `--key`, `--format`)
2. **Environment Variables** (`SCANDRIX_SERVER_URL`, `SCANDRIX_API_KEY`)
3. **Local Repository Config** (`.scandrix/config.json`)
4. **Global Developer Config** (`~/.scandrix/config.json`)
5. **Secure Credentials Store** (`~/.scandrix/credentials.json` with `0600` permissions)

---

## 📋 Implementation Plans & Design History
All architectural extraction design docs and QA checklists are located in:
- [`docs/cli/plans/`](cli/plans/)
