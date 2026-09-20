---
name: scandrix-sso-e2e
description: Use when the user wants to validate the ScanDrix enterprise SSO flow end-to-end (cookie-domain isolation for self-hosted multi-tenant setups, SAML 2.0 round-trip via Keycloak, OAuth 2.0 / OIDC flows), confirm SSO tests pass after code changes, or regression-check before merging code touching auth controllers, session issuance, or SAML metadata handlers.
---

# ScanDrix Enterprise SSO End-to-End Test

## Overview

Drives the complete multi-tenant Enterprise SSO regression suite: unit/integration tests + production container runtime smoke tests (multi-tenant cloud + isolated on-prem) + full browser SAML round-trip via Keycloak + Caddy + mkcert. Reports back which layers passed and which failed with the exact failure surface and security headers.

---

## When to Use

- User asks to validate that enterprise SSO is working after code or configuration changes.
- Validating that both SaaS deployments (`*.scandrix.io` / `*.scandrix.dev`) and isolated customer self-hosted deployments (`*.corp.company.com`) authenticate cleanly.
- Pre-merge check on changes touching `internal/auth/sso/`, `internal/auth/oauth/`, session cookie issuance, or the web `/sso-callback` route.
- After upgrading SAML libraries, JWT signing modules, or session encryption keys.

## When NOT to Use

- Routine unit testing during active editing → run fast unit tests directly without booting external IdP containers:
  ```bash
  go test -v -run TestSSO ./internal/auth/sso/...
  ```
- General code review of auth handlers — this skill executes live integration tests, it does not analyze source code.

---

## Test Depth & Workflow

### 1. Choose Test Depth

- **Quick (Default)**: Unit + integration tests + session token domain derivation checks (~10s). Catches domain calculation regressions, tenant ID context mismatches, and token signing flaws.
- **Full**: Boots the complete local SSO stack (Keycloak IdP + Caddy reverse proxy + ScanDrix API + Web), executes real SAML assertion posts via Playwright, and asserts browser cookie storage (~3 min).

### 2. Quick Path

Run the fast Go integration suite:
```bash
go test -v -count=1 ./internal/auth/sso/... ./internal/auth/oauth/...
```

Expected output:
```text
=== RUN   TestSAMLMetadataIngestion
--- PASS: TestSAMLMetadataIngestion (0.01s)
=== RUN   TestCookieDomainDerivation_CloudVsSelfHosted
--- PASS: TestCookieDomainDerivation_CloudVsSelfHosted (0.00s)
=== RUN   TestOAuthStateStore_CSRFReplayPrevention
--- PASS: TestOAuthStateStore_CSRFReplayPrevention (0.01s)
PASS
```

### 3. Full Path (Live Keycloak + Caddy + Playwright)

#### Pre-Flight Checks
Verify before launching the browser/container layer:
```bash
# 1. Backing database and Redis up?
docker ps --format '{{.Names}}' | grep -qE '^(postgres|redis)$' || echo "WARN: Start backing services first"

# 2. Local trusted CA (mkcert) available for wildcard *.scandrix.lvh.me?
mkcert -CAROOT >/dev/null 2>&1 || echo "ERROR: install with 'sudo apt-get install libnss3-tools && brew/curl mkcert'"
```

#### Launch Local Keycloak IdP & Caddy Stack
1. Start local Keycloak realm seeded with test IdP client:
   ```bash
   docker run -d --name keycloak-sso-e2e -p 8443:8443 \
     -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin \
     quay.io/keycloak/keycloak:24.0.0 start-dev
   ```
2. Ingest ScanDrix SP (Service Provider) metadata into Keycloak:
   - SP Entity ID: `https://api.scandrix.lvh.me/auth/sso/saml/metadata`
   - ACS (Assertion Consumer Service) URL: `https://api.scandrix.lvh.me/auth/sso/saml/callback`
   - NameID Format: `emailAddress`

#### Drive Browser Round-Trip via Playwright / Chrome
When running with Playwright:
1. `browser_navigate https://api.scandrix.lvh.me/auth/sso/login/<org-uuid>`
2. Follow redirect to Keycloak login page:
   - Fill credentials: `sso-test-user@scandrix-eval.com` / `TestSso!2026`
3. Click "Sign In":
   - Keycloak signs the SAML response with its private X.509 certificate.
   - Posts SAML assertion back to `/auth/sso/saml/callback`.
4. Browser redirects to `https://app.scandrix.lvh.me/dashboard`.
5. Verify browser cookies via DevTools or API logs:
   - Verify `scandrix_session` is present.
   - Assert cookie flags: `HttpOnly; Secure; SameSite=Lax`.
   - Assert cookie domain is scoped to `.scandrix.lvh.me` (or parent domain), NEVER wide-open public suffixes (`.com`, `.io`).

---

## Negative Security & Edge Scenarios

Always execute these negative test vectors when modifying SSO logic:

1. **Tampered SAML Assertion**:
   - Alter 1 character in the XML `<Assertion>` payload before submitting.
   - *Expected Outcome*: Server returns `401 Unauthorized` with `Signature verification failed`. Must not panic.
2. **Expired Assertion**:
   - Submit assertion with `NotOnOrAfter` timestamp in the past.
   - *Expected Outcome*: `401 Unauthorized` with `SAML assertion has expired`.
3. **Replay Attack**:
   - Re-submit the exact same valid SAML response twice.
   - *Expected Outcome*: Second request rejected with `Assertion ID already consumed (replay detected)`.
4. **Tenant Impersonation / Organization Mismatch**:
   - User signs in via Org A's IdP but specifies Org B's callback URL.
   - *Expected Outcome*: Strict rejection. Issuer must match organization's configured IdP Entity ID in database.

---

## Common Failure Modes & Troubleshooting

* **"Invalid redirect_uri" from IdP**:
  - The ACS URL configured in Keycloak does not match the URL emitted by the backend.
  - *Fix*: Check `SCANDRIX_API_URL` environment variable. Ensure scheme is `https://` when behind reverse proxy.
* **Cookie not stored by browser after successful SAML post**:
  - Browser rejects cookie if `Secure=true` is sent over plain `http://` without TLS.
  - *Fix*: Use Caddy or mkcert with trusted local certificates (`*.lvh.me`).
* **Cross-Subdomain Session Loss**:
  - When navigating from `app.scandrix.lvh.me` to `api.scandrix.lvh.me`, session disappears.
  - *Fix*: Cookie domain must include leading dot (`Domain=.scandrix.lvh.me`). If domain is omitted, modern browsers restrict the cookie to the exact origin host only.

---

## Hard Rules

- **Never** disable signature verification or certificate validation in production mode.
- **Never** accept `SameSite=None` without `Secure=true`.
- **Never** permit public suffix scope domains (`.com`, `.io`, `.net`, `.org`) in session cookie domains. Minimum 2 domain labels required.
- **Never** commit private SAML signing keys or IdP test secrets to version control.
