# Frontend Architecture (scandrix-dashboard)

Next.js 14 App Router dashboard (port 3001). 11 routes, 66 components, no Auth.js/React-Query — session is a locally verified JWT cookie, data flows through one API client. The marketing/docs site (`scandrix-website`, port 3000) is separate and out of scope here.

## 1. Route map

| Route | File | Purpose | Data owner |
|---|---|---|---|
| `/` | `app/(dashboard)/page.tsx` | Cockpit: metrics, DORA, code health, hotspots, PRs, CLI reviews | `api-client` + `CockpitClientView` |
| `/pull-requests`, `/pull-requests/[id]` | `…/pull-requests/page.tsx` | PR list + review studio (diff, findings, progress) | `ReviewStudioClient`, `review-store.tsx` |
| `/cli-reviews` | `…/cli-reviews/page.tsx` | CLI execution history | `api-client` |
| `/tokens` | `…/tokens/page.tsx` | Token usage & cost | `TokenUsageDashboard` |
| `/byok` | `…/byok/page.tsx` | Provider keys, model routing, spend limits | `ByokClientView` |
| `/rules` | `…/rules/page.tsx` | Rule studio, dry-run, test console | `RuleStudioClientView` |
| `/integrations` | `…/integrations/page.tsx` | SCM/PM connections, repos, webhook health | `IntegrationsClientView` |
| `/organization` | `…/organization/page.tsx` | Teams & RBAC, CLI keys, audit logs, enterprise SSO | `OrganizationClientView` (4 tabs) |
| `/settings` | `…/settings/page.tsx` | Members + CLI tokens | server component |
| `/sign-in`, `/sign-up` | `app/sign-in|sign-up/page.tsx` | Auth forms | `SignInForm`, `SignUpForm` |
| `/api/proxy/api/[...path]` | `app/api/proxy/…` | Browser → backend proxy (injects bearer) | edge-ish route handler |
| `/api/auth/{login,register,oauth/[provider]}` | `app/api/auth/…` | Auth endpoints | route handlers |

## 2. Auth & session flow

1. Sign-in posts to `/api/auth/login`, which tries the backend (`SCANDRIX_API_URL/auth/login`) and falls back to a locally minted session.
2. Session = `scandrix_token` JWT + `scandrix_session` user JSON cookies; `middleware.ts` verifies the token and redirects `/login → /sign-in`.
3. `(dashboard)/layout.tsx` re-verifies server-side before rendering.
4. Browser → backend calls go through `/api/proxy/api/v1/*` with the cookie's bearer injected server-side; server components call `ScanDrixApi` directly (`src/lib/api-client.ts`, `cache: no-store`).

**Known deviations from the security baseline (must be fixed before EE is sold):** cookies are `httpOnly: false`; a dev-mode auth bypass exists; the OAuth route mints a stub session without an upstream exchange; the proxy doesn't forward `PATCH`. Tracked in `security/threat-model.md` T5 and the EE gap list — not to be presented as production auth.

## 3. Conventions

- **One client:** `src/lib/api-client.ts` is the only place that talks to the backend. Components never `fetch` directly. Every method catches and returns an empty/zero fallback (visible as empty states, never as fake data).
- **Types:** `src/types/*` (byok, capabilities, organization, rules, integrations) mirror backend DTOs; `types/capabilities.ts` is the entitlement snapshot the UI gates on.
- **State:** server components fetch; interactive islands use local `useState`/`useReducer` + a small store (`pr-review/review-store.tsx`). No global cache layer yet.
- **Styling:** Tailwind with CSS-variable theme tokens + brand palette; dark by class. Primitives in `components/ui/`.
- **Entitlement UI mirrors backend gates:** hide/label gated controls from `capabilities` — but the backend `RequireFeature` middleware is the enforcement point, never the UI (`api/middleware/feature_gate.go`).
- **Empty vs error:** components render `EmptyState`/`Skeleton`; failures degrade to empty, never to invented rows.

## 4. Enterprise surfaces in the UI (honest status)

| Surface | State |
|---|---|
| Enterprise SSO config card | Real UI, **implemented backend** (`/sso-config` backed by `sso_configs` table and `SSOConfigRepository`) |
| Audit logs table | Real viewer, reads the user-activity log; export/retention UI **missing** |
| Teams & RBAC | Invite + roles (member/admin/viewer); role editing, per-repo scope UI, SCIM UI **missing** |
| Team CLI keys | Full lifecycle (create/reveal-once/revoke) |
| Billing/subscription | **Missing** (sidebar links to an unhandled query param) |
| License activation | **Missing** (no key input, no expiry/seat display) |
| SSO sign-in button | **Missing** (config exists, login path doesn't consume it) |

The dashboard must not be demoed as if these exist; the API reference marks the same rows STUB/SPECCED.
