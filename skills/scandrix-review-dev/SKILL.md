---
name: scandrix-review-dev
description: Use when the user explicitly asks to run ScanDrix CLI against a local development or QA API server (e.g. localhost:8080, custom SCANDRIX_API_URL).
---

# ScanDrix Review (Dev / Local Backend)

## Goal

Run the local build of ScanDrix CLI pointed against a local development API server (`http://localhost:8080` or staging environment) rather than production cloud endpoints.

## When to Use

- Explicit developer requests mentioning local backend, `localhost:8080`, `dev API`, `QA API`, or custom `SCANDRIX_SERVER_URL`.
- Testing newly implemented review heuristics, API changes, or mock LLM providers locally before deploying.

## Workflow

1. Ensure the local backend API server is running (`go run ./cmd/api` or `docker compose up`).
2. Pass the `--server` flag or set `SCANDRIX_SERVER_URL` in the environment:

```bash
scandrix review --server http://localhost:8080 --prompt-only
scandrix review --staged --server http://localhost:8080 --agent
```

3. Parse output and apply remediations as standard.
