# ScanDrix — Local Setup & Runbook

This is the verified path to a working dashboard. Every command below was run
against this repository; every claim about a default port or required variable
was read from source, not assumed.

---

## 0. Prerequisites

| Tool | Version | Check |
|---|---|---|
| Go | 1.25.3 | `go version` |
| Node | 20+ (verified on 24.12) | `node -v` |
| npm | 11+ | `npm -v` |
| Docker | any | `docker --version` |

---

## 1. Backend

### 1.1 Environment

The backend reads `/.env` (and tolerates a missing `SCANDRIX_ENCRYPTION_KEY`
outside production). Only these matter for a local run:

```
APP_ENV=development          # production/staging add startup validation
PORT=8080
DATABASE_URL=…               # required
JWT_SECRET=…                 # required in production/staging
REDIS_URL=redis://localhost:6379   # optional; the API degrades without it
RABBITMQ_URL=…               # optional locally; a remote URL is fine
```

> **JWT_SECRET must be byte-identical to the dashboard's.** The dashboard
> verifies the session token itself (`scandrix-dashboard/src/lib/auth-session.ts`)
> before forwarding any request. If the two secrets differ, *every* API call
> fails with 401 while the backend itself looks healthy.

`REDIS_URL` is genuinely optional — `cmd/api/main.go:74-83` logs
`Redis connection deferred` and continues. You lose distributed caching and
cross-replica locks, nothing else.

### 1.2 Dependencies

```bash
docker compose up -d postgres redis
```

RabbitMQ is optional locally; if `RABBITMQ_URL` points at a remote broker the
local container is not needed.

### 1.3 Migrate, then run

```bash
go run ./cmd/migrate     # applies migrations/*.sql
```

### 1.4 The three processes

```bash
go run ./cmd/api         # REST API            → :8080
go run ./cmd/worker      # review execution    → no port
go run ./cmd/webhooks    # webhook intake      → :8081
```

**The worker is not optional for the dashboard.** Reviews are produced by the
worker; the API only records their results. With the API alone, the cockpit,
pull-request list and review studio render correctly but permanently empty —
which is the honest state, not a wiring bug.

### 1.5 Verify

```bash
curl -i http://localhost:8080/api/v1/system/health
```

---

## 2. MCP Manager microservice

The MCP Manager is a **separate binary**. It is not mounted into the main
API's router, so `SCANDRIX_API_URL` cannot reach it. It serves integration CRUD
at `/mcp/*` and persists to the `"mcp-manager"` schema (migration `022`).

```bash
go run ./cmd/mcp-manager          # → :3101 (API_MCP_MANAGER_PORT / MCP_MANAGER_PORT)
```

It requires `DATABASE_URL` (it refuses to guess one) and
`API_MCP_MANAGER_ENCRYPTION_SECRET` for credential encryption.

The dashboard reaches it through `/api/mcp-proxy/*`, which forwards to
`SCANDRIX_MCP_MANAGER_URL`. **Leave that variable unset and the MCP gateway
surface reports "unknown" — it does not fabricate an empty or populated list.**

---

## 3. Dashboard

### 3.1 Environment

Copy `scandrix-dashboard/.env.example` to `.env.local`:

```
SCANDRIX_API_URL=http://localhost:8080
SCANDRIX_MCP_MANAGER_URL=http://localhost:3101
NEXT_PUBLIC_APP_URL=http://localhost:3001
NEXT_PUBLIC_WEBSITE_URL=http://localhost:3000
JWT_SECRET=<same value as the backend>
NODE_ENV=development
```

`NEXT_PUBLIC_APP_URL` is required, not cosmetic: `hasValidOrigin()` in
`src/lib/backend-auth.ts` fails closed without it, and every state-changing
request through the proxy is rejected with 403.

### 3.2 Install and run

```bash
cd scandrix-dashboard
npm install
npm run dev            # → :3001
```

### 3.3 Signing in

Use a real account. The API is the only thing that can authenticate a user, so
there is no local bypass.

For a seeded development account, set both of these in `.env.local` and a
**Development entry → Enter cockpit** control appears on `/sign-in`:

```
DEV_LOGIN_EMAIL=
DEV_LOGIN_PASSWORD=
```

The button only renders when the route reports itself available. It is not an
authentication bypass: the backend still performs the real credential check on
the configured account, and the route answers 404 outside
`NODE_ENV=development`.

---

## 4. Verification gate

Every change must leave the tree green:

```bash
go build ./...                       # backend compiles
cd scandrix-dashboard
npx tsc --noEmit                     # zero type errors
npx next build                       # production bundle succeeds
npx vitest run                       # full suite
```

Contract drift check — no frontend call may target a route the router does not
register:

```bash
cd <repo-root>
grep -rn 'Mount("' internal/api/router.go     # what the backend mounts
grep -rhoE '"/api/v1/[^"]*"' scandrix-dashboard/src | sort -u
```

---

## 5. Known-empty states (not defects)

These render empty or "unknown" because no backend source exists. They are
deliberate: the alternative was a plausible-looking fabricated value.

| Surface | Why it is empty |
|---|---|
| Commit list on a review | The execution endpoint stores no commit list |
| Per-gateway tool counts | Populated only by a discovery run; `null` means not probed |
| Gateway latency | No probe endpoint is exposed, so no round trip has been measured |
| Seat assignment / revocation | No server-side seat operation exists |
| Tax invoices | No billing-invoices endpoint exists |
| External issue linking | No commit, push or issue-create endpoint exists |
| Suggestion "apply" | Copy-to-clipboard only; the API cannot write to a repository |

---

## 6. Troubleshooting

| Symptom | Cause |
|---|---|
| Every API call returns 401, backend is healthy | Dashboard `JWT_SECRET` ≠ backend `JWT_SECRET`, or the dashboard has none set |
| Writes fail with 403 | `NEXT_PUBLIC_APP_URL` unset or does not match the origin you are browsing from |
| All metrics empty | Worker not running, or no repository connected to a provider |
| `/api/v1/*` returns 502 "Backend service unreachable" | API not started, or `SCANDRIX_API_URL` wrong |
| MCP page shows "The integration list could not be loaded" | MCP Manager not running, or `SCANDRIX_MCP_MANAGER_URL` unset |
| `next build` fails on `.next/types/...` referencing a deleted route | `rm -rf .next/types` and re-run |

---

## 7. Repository conventions

- `.env`, `.env.*` and `*.pem`/`*.key` are gitignored. `.env.example` and
  `.env.production.template` are the committed shapes.
- A derived metric is either computed from real data or returned as `null` with
  a machine-readable reason in `unavailable`. Never a constant, never a
  substituted zero.
- A test that asserts a fallback *exists* is a test that passes precisely when
  the code is lying. New tests assert the real shape and assert that failures
  **reject**.
- Client-side permission checks are UX only. The Go backend is the enforcement
  point for every write.
