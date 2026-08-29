# MASTER RULES: Secrets, Real Integrations & Enterprise Security

You are an AI coding agent working on this project. The rules below are permanent operating instructions, not one-time advice — apply them to every file you write, every feature you implement, and every piece of existing code you touch, without being reminded. They outrank speed, convenience, or making something merely look finished. If a request conflicts with a rule here, say so plainly and propose a compliant path instead of silently picking one side.

## 0. Operating Principles
- Security, correctness, and honesty about what actually works outrank speed or "looks done."
- Before writing new code, check what already exists — `package.json`/`requirements.txt`/`go.mod`, the current `.env.example`, existing routes/models, existing folder structure — so you extend the real stack instead of guessing, duplicating, or inventing a different one.
- If you're not sure whether something is a secret, treat it as one.

## 1. Secrets & Environment Variables — Non-Negotiable
1.1 Every API key, token, password, connection string, webhook secret, or credential lives in `.env` (local dev) or the platform's secret manager (staging/production) — never in source code, config files that get committed, comments, or docs.
1.2 Never write a real secret value into a `.js`, `.ts`, `.py`, `.json`, `.yaml`, `.html`, or any tracked file. Read it at runtime: `process.env.KEY_NAME` (Node), `os.environ["KEY_NAME"]` (Python), or the equivalent for the stack in use.
1.3 Know your framework's client-exposure rules before adding a variable:
- Next.js — only `NEXT_PUBLIC_*` reaches the browser.
- Vite — only `VITE_*` is exposed via `import.meta.env`.
- Create React App — only `REACT_APP_*` is inlined at build time.
Anything without one of these prefixes stays server-only. A secret key must never carry a public/exposable prefix. If a value really is safe to expose (a Stripe publishable key, a public map token, an analytics ID), it isn't a secret — label it as public config and keep it clearly separate from real secrets.
1.4 Third-party APIs that require a secret key are called only from backend code. The frontend calls your own backend endpoint; the backend calls the third-party service using `process.env`. A secret key should never appear in the browser's network tab, page source, or bundle.
1.5 `.env`, `.env.local`, `.env.*.local`, and anything else holding real secrets go in `.gitignore` from the first commit. Provide a `.env.example` with every required variable name and an empty or placeholder value — never a real one — so the project is reproducible without leaking anything.
1.6 Validate required environment variables at startup and fail loudly if one is missing, instead of letting the app boot into a broken or insecure state.
1.7 Never `console.log`/print a secret, and never let one appear in an error message, stack trace, or crash report.
1.8 In staging/production, prefer a real secret manager (AWS Secrets Manager/Parameter Store, Google Secret Manager, Azure Key Vault, HashiCorp Vault, Doppler, Infisical, or the host's built-in encrypted env vars) over a flat file where the platform supports it. `.env` is a local-dev convenience, not a production secret store.
1.9 If a secret is ever committed, logged, or pasted somewhere public, treat it as compromised: rotate/revoke it at the provider immediately. Deleting the line doesn't remove it from git history.

### Examples:
```bash
# .env.example — commit this file; never commit .env itself
DATABASE_URL=
STRIPE_SECRET_KEY=
STRIPE_WEBHOOK_SECRET=
JWT_SECRET=
NEXT_PUBLIC_API_BASE_URL=https://api.example.com   # not a secret — safe to expose
```

**Never:**
```js
// frontend — secret key visible in every request
const res = await fetch(`https://api.stripe.com/v1/charges?key=sk_live_51H...`);
```

**Always:**
```js
// frontend calls your own backend
await fetch("/api/checkout", { method: "POST", body: JSON.stringify(cart) });

// backend/routes/checkout.js — secret stays server-side
const stripe = require("stripe")(process.env.STRIPE_SECRET_KEY);
```

## 2. Build It Real — No Fake Data, No Mock Stubs
2.1 "Done" means the whole chain actually works: frontend → your backend endpoint → real database query or real third-party call → real response rendered in the UI. Code that only looks wired up but returns invented data isn't done.
2.2 Don't hardcode sample values or invented "success" responses and present them as if they came from the real system. If a response is faked instead of coming from a real DB write/API call, say so explicitly — don't let it pass as finished.
2.3 Don't fabricate a plausible-looking API key, database URL, account ID, or webhook secret so code compiles or runs. A fake value that looks real is worse than an obvious placeholder — it fails silently or misleads later.
2.4 If a credential, endpoint, or service you need isn't available yet: stop and ask. Name exactly which environment variable, which service, and where to get it. Don't guess, invent, or silently ship a stub disguised as the real thing.
2.5 Before calling a feature complete, actually exercise it — hit the endpoint, confirm the row was written/read in the real (or sandbox) database, confirm the third-party call returns a real response. Report what you actually tested, not what "should" work.
2.6 Remove debug output, temporary routes, and commented-out mock code before shipping. If something is intentionally still a stub, label it clearly (`// TODO: needs STRIPE_WEBHOOK_SECRET from user`) rather than leaving it mixed in silently with finished code.

## 3. Definition of Done — Every New Feature
- [ ] Frontend calls a real backend endpoint — no mocked fetch, no hardcoded response
- [ ] Backend performs a real DB query or real third-party call, not a stub
- [ ] Every secret involved is read from `.env`/secret manager — none hardcoded, none in the client bundle
- [ ] `.env.example` updated with any new variable name (name only, no real value)
- [ ] Server-side input validation and authorization checks are in place, not just hidden UI
- [ ] Errors are handled and surfaced without leaking internals (stack traces, DB errors, file paths)
- [ ] No leftover logs of sensitive data, no dead mock code paths
- [ ] Any still-missing credential or config is explicitly flagged to the user, not silently faked
- [ ] Critical paths (auth, payments, writes) have at least a basic automated test where feasible
- [ ] The feature was actually run once, end to end, and the result reported honestly

## 4. Frontend ⇄ Backend Boundary & Access Control
4.1 Never trust the client. Anything the frontend does for UX — form validation, hiding a button, role-based UI — gets re-checked on the backend before the action executes. Hiding a button is not access control.
4.2 CORS is an explicit allow-list of real origins, never `*` combined with credentialed requests.
4.3 Authorization is enforced per request, on the backend, based on the authenticated session/token — never inferred from what the frontend claims about the current user.
4.4 Every query that fetches or mutates a specific resource (an order, a document, a record) is scoped to what the authenticated caller actually owns or has permission to see — check it on the backend. Never rely on an ID being "hard to guess" as a security control (this is the classic insecure-direct-object-reference bug).
4.5 Rate-limit and validate every public-facing endpoint, especially auth, payments, and anything that writes to the database.

## 5. Enterprise Security Baseline

### 5.1 Authentication & Sessions
- Hash passwords with bcrypt or argon2 (argon2id) at an appropriate cost factor — never MD5/SHA1/plaintext, never a homemade scheme.
- For JWTs: short-lived access tokens with refresh-token rotation, signature and expiry validated on every request, `alg: none` never accepted.
- Store session tokens in httpOnly, Secure, SameSite=Lax/Strict cookies rather than localStorage, where XSS exposure is a concern.
- Support MFA where the product calls for it; lock out or back off after repeated failed logins.

### 5.2 Data Protection
- TLS everywhere — HTTPS only, HSTS enabled. No plaintext transport for credentials or personal data.
- Encrypt sensitive data at rest where appropriate (payment details, government IDs, health data).
- Never store raw card numbers, CVV, or full card data yourself — use your payment processor's tokenization (Stripe, Braintree, etc.) and keep only the token/customer ID.
- Least-privilege access for every DB user, service account, and cloud IAM role — scope to only what that service actually needs.

### 5.3 Input/Output Handling
- Parameterized queries or a real ORM everywhere — never string-concatenated SQL.
- Escape/sanitize user-generated content before rendering it; rely on the framework's escaping rather than manual string surgery, to prevent stored/reflected XSS.
- Validate file uploads server-side: real type checks (not just extension), size limits, storage outside the web root or via signed URLs, malware scanning for anything user-uploaded and later served to others.

### 5.4 HTTP & Network Hardening
- Send Content-Security-Policy, `X-Content-Type-Options: nosniff`, a frame-ancestors/`X-Frame-Options` policy, and Referrer-Policy headers.
- CSRF protection on any cookie-authenticated, state-changing request.

### 5.5 Third-Party Integrations & Webhooks
- Call the real API with real credentials from `.env`; handle that service's actual error responses (rate limits, auth failures) instead of assuming success.
- Verify webhook signatures (Stripe, GitHub, Twilio, etc.) using the webhook secret before trusting an inbound payload — never process an unverified webhook as authentic.
- Confirm with the user which environment's credentials apply — sandbox/test vs. live/production — before wiring a task to one or the other; mixing them causes real damage (real charges in a test run, or test data in production).

### 5.6 Dependencies & Supply Chain
- Commit lockfiles. Run `npm audit`/`pip-audit`/`safety` (or equivalent) regularly and keep dependencies patched.
- Use Dependabot/Renovate (or equivalent) so vulnerable packages get flagged automatically.

### 5.7 Logging & Error Handling
- Log enough to debug and audit (who did what, when) without ever logging secrets, passwords, full card numbers, or other sensitive data in plaintext.
- Return generic error messages to end users in production; keep stack traces and internals in server-side logs only.
- Fail closed — if a permission check or validation step errors, deny the action rather than defaulting to allow.

### 5.8 Database Security
- Least-privilege DB users (the app's DB user is not a superuser); separate read/write credentials where it matters.
- Encrypted connections to the database, encrypted backups, and an actual backup/restore process — not just an aspiration.

### 5.9 Infrastructure & Environments
- Dev/staging/prod are isolated with separate credentials — a prod secret never lives in a dev `.env` or test suite.
- CI/CD secrets live in the CI platform's encrypted secret store (GitHub Actions secrets, GitLab CI/CD variables, etc.), never printed to build logs or embedded in pipeline YAML.
- Secret-scanning in CI (gitleaks, truffleHog, or equivalent) as a safety net for anything that slips past `.gitignore`.

### 5.10 Compliance Awareness
- If the product handles personal, financial, or health data, ask the user which regime applies (GDPR, CCPA, HIPAA, PCI-DSS, SOC 2, etc.) and follow its specific requirements rather than assuming general best practice alone is sufficient.

## 6. Git & Version Control Hygiene
- `.gitignore` includes `.env`, `.env.*`, `*.pem`, `*.key`, and any local credential file from the first commit.
- Never commit a secret "just for now" — history keeps it forever unless the repo is rewritten, which is disruptive and easy to forget.
- If you find an existing hardcoded secret while working in a codebase, flag it immediately and propose moving it to `.env`/a secret manager, even if that wasn't the task you were asked to do.

## 7. How to Communicate With the User
- Whenever a feature needs a new environment variable, state its exact name, what service it's for, and where to get it — then wait for it to be added rather than inventing a value.
- Don't ask the user to paste a real secret into chat if it can be avoided — ask them to add it directly to their local `.env` instead.
- Be explicit about what's real vs. still pending: "this is fully wired and tested against the real database" and "this is still stubbed, waiting on X_API_KEY" are both fine to say — blurring the two is not.

## 8. Quick-Scan — Absolute Nevers
- Never hardcode a secret in source, config, comments, or docs.
- Never let a secret reach frontend/client-bundle code.
- Never call a third-party API requiring a secret key directly from the browser.
- Never fabricate a fake key or response and present it as a working integration.
- Never leave mock data wired into a "finished" feature without saying so.
- Never trust client-side validation or authorization alone.
- Never commit `.env` or any real credential file.
- Never log a secret or full sensitive-data value.
- Never silently ship with a missing credential — ask instead.
