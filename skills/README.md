# ScanDrix Agent Skills

This directory contains the autonomous AI agent skills shipped with the ScanDrix platform and CLI.
It implements a **Two-Tier Architecture**:
1. **Tier 1: Core Security Guard** (Always-active, zero-tolerance enforcer for secrets, injection, fail-closed auth).
2. **Tier 2: Specialized Deep-Dive Modules & SDLC Workflow Skills** (Loaded dynamically or just-in-time when auditing specific domains like Crypto, Cloud/Infra, Web AppSec, Supply Chain, Performance, or SSO).

---

## 1. Two-Tier Security Architecture

### Tier 1: Core Guard (Always Active)
- `scandrix-security-core`
  - Zero-tolerance secret scanner, parameterized query enforcer, fail-closed authorization checker, and sensitive error masking.

### Tier 2: Deep-Dive Domain Modules (On-Demand / Glob-Matched)
- `scandrix-sec-crypto`
  - Cryptographic standards (AES-256-GCM, Argon2id, ChaCha20, CSPRNG enforcement, no custom crypto).
- `scandrix-sec-infra`
  - Cloud and container hardening (rootless containers, distroless images, read-only rootfs, drop capabilities, security headers).
- `scandrix-sec-input-web`
  - Web application security (strict schema validation, context-aware XSS escaping, magic byte file upload verification, cookie security).
- `scandrix-sec-supplychain`
  - Dependency integrity (lockfile enforcement, `govulncheck`, `trivy fs`, typosquatting prevention).

---

## 2. SDLC & Code Review Workflow Skills

- `scandrix-review`
  - Run local ScanDrix security and quality code review for workspace changes using the installed CLI. Emits prompt-friendly markdown with `--prompt-only` or JSON with `--agent`.
- `scandrix-review-dev`
  - Run the local ScanDrix CLI build against a local/dev backend API for debugging or offline development. Includes `scripts/run-local-cli.sh`.
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
- `scandrix-sso-e2e`
  - Automated enterprise SAML 2.0 and OAuth SSO end-to-end multi-tenant verification.
- `scandrix-perf-debug`
  - Front-to-back performance profiling, database index analysis, and latency bottleneck diagnosis.
- `hunk-review`
  - Drive a live Hunk diff review session: inspect, navigate, reload, and leave inline comments via `hunk session ...`.

---

## Trigger Map (Recommended)

- **General Coding & Code Generation**:
  - Always enforce `scandrix-security-core`.
- **Touching crypto, tokens, hashing, JWT, or passwords**:
  - Load `scandrix-sec-crypto`.
- **Touching Dockerfiles, Kubernetes manifests, or cloud infrastructure**:
  - Load `scandrix-sec-infra`.
- **Touching web routes, forms, file uploads, or API endpoints**:
  - Load `scandrix-sec-input-web`.
- **Modifying dependencies (`go.mod`, `package.json`, lockfiles)**:
  - Load `scandrix-sec-supplychain`.
- **User mentions `review`, `commit`, `push`, `open PR`, `merge`, `quality gate`, `security scan`**:
  - Prefer `scandrix-review` (or `scandrix-review-dev` for local backend development).
- **User mentions `business validation`, `acceptance criteria`, `task requirements`**:
  - Use `scandrix-business-rules-validation`.
- **User asks to fetch, triage, or apply PR suggestions**:
  - Use `scandrix-pr-suggestions-resolver`.
- **User asks to manage centralized configuration or sync rules**:
  - Use `scandrix-centralized-config`.
- **User asks to create, modify, inspect, or test custom review rules**:
  - Use `scandrix-rules`.
- **User asks why code is written a certain way or checks architectural decisions**:
  - Use `scandrix-trace`.
- **User mentions SSO testing or SAML authentication verification**:
  - Use `scandrix-sso-e2e`.
- **User diagnoses platform latency, slow screens, or query bottlenecks**:
  - Use `scandrix-perf-debug`.
- **User has a Hunk or TUI diff session open**:
  - Use `hunk-review`.

---

## Agent Installation & Sync

- `scandrix skills list` — list all bundled skills and descriptions.
- `scandrix skills install` — sync bundled skills into detected local agent roots (`.cursor/rules/`, `.claude/`, `.agents/skills/`).
- `scandrix skills resync` — re-synchronize all managed skills to latest versions.
- `scandrix skills uninstall` — cleanly remove managed skills.
- `scandrix skills prompt` — generate XML (`<available_skills>`) prompt payload for LLM system prompt injection.
- `scandrix skills prompt --json` — generate JSON prompt payload for tooling and MCP servers.
