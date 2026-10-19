# ScanDrix Bruno API Collection & Automated Contract Tests

This directory houses the comprehensive, zero-hardcoded **Bruno** (`.bru`) API collection and automated test suites for the ScanDrix enterprise platform.

---

## 1. Zero-Hardcoding Principles

1. **Environment-Driven URLs:**
   All host URLs are parameterized via `{{apiBaseUrl}}`, `{{webhooksBaseUrl}}`, and `{{workerBaseUrl}}`. No domain or IP is hardcoded in any request.
2. **Dynamic Authentication Chaining:**
   The `02-auth-lifecycle/01-register` or `02-login` request automatically extracts the returned JWT and dynamically sets `authToken` in the active runtime context. All subsequent requests consume `Authorization: Bearer {{authToken}}`.
3. **Dynamic HMAC Webhook Signatures:**
   Webhook requests (GitHub `X-Hub-Signature-256`, Razorpay `X-Razorpay-Signature`) dynamically compute their cryptographic HMAC-SHA256 digests in pre-request scripts from the actual request body and environment secret, guaranteeing test payload freshness.
4. **Idempotent Test Isolation:**
   Dynamic UUIDs and timestamp nonces prevent primary key conflicts across repetitive runs.

---

## 2. Directory Layout

```
bruno/
├── bruno.json                               # Bruno Collection definition
├── environments/
│   ├── local.bru                            # http://localhost:8080 / 8081 / 8082
│   ├── staging.bru                          # https://staging-api.scandrix.dev
│   └── production.bru                       # https://api.scandrix.dev
├── 01-system-health/                        # Health, liveness & Prometheus metrics
│   ├── api-health.bru
│   ├── webhooks-health.bru
│   ├── worker-health.bru
│   └── prometheus-metrics.bru
├── 02-auth-lifecycle/                       # Registration, login, refresh, RFC 8628 CLI device flows
│   ├── 01-register.bru
│   ├── 02-login.bru
│   ├── 03-refresh-token.bru
│   ├── 04-device-initiate.bru
│   └── 05-device-poll.bru
├── 03-workspaces-and-teams/                 # Multi-tenant workspace, teams & CI token management
│   ├── 01-get-workspace.bru
│   ├── 02-list-teams.bru
│   ├── 03-create-team.bru
│   └── 04-list-cli-tokens.bru
├── 04-code-reviews-and-stream/              # Pull requests, findings, cockpit & live SSE event streams
│   ├── 01-list-pull-requests.bru
│   ├── 02-subscribe-sse-events.bru
│   ├── 03-get-findings.bru
│   └── 04-get-cockpit-metrics.bru
├── 05-webhooks-intake/                      # SCM webhooks with dynamic cryptographic verification
│   ├── 01-github-webhook.bru
│   ├── 02-gitlab-webhook.bru
│   ├── 03-bitbucket-webhook.bru
│   └── 04-azure-webhook.bru
├── 06-billing-and-subscriptions/            # Plan tiers & Razorpay payment webhook simulation
│   ├── 01-list-plans.bru
│   └── 02-razorpay-webhook.bru
├── 07-pentest-and-compliance/               # Pentest audit reports & compliance mapping
│   └── 01-list-pentest-reports.bru
└── 08-scim-provisioning/                    # SCIM 2.0 enterprise user/group endpoints
    ├── 01-scim-get-users.bru
    └── 02-scim-get-groups.bru
```

---

## 3. How to Run

### In Bruno Desktop GUI:
1. Open [Bruno](https://www.usebruno.com/).
2. Click **Open Collection** and select the `/home/tarun/Videos/kodus-ai/ScanDrix/bruno` folder.
3. Select the `local`, `staging`, or `production` environment in the top-right environment picker.
4. Run requests or use the **Collection Runner** to run all tests in sequence.

### In Terminal / CI Automation:
```bash
# Run via Makefile
make test-bruno

# Or directly with Bru CLI
cd bruno && npx --yes @usebruno/cli run --env local
```
