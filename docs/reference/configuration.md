# ScanDrix Configuration Reference

Single table of every environment variable. Source of truth order: `internal/config.Load()` (fail-fast validation) → this doc → `.env.example` (names only). If they disagree, code wins and this doc must be fixed in the same PR.

Conventions: **Required** = boot fails without it (in the stated env). **Rotation** = what breaks/overlaps when you change it. **Air-gap** = relevance when `AIR_GAPPED` enforcement is on (Phase 1) or under deployment firewalling today.

## Runtime

| Variable | Required | Default | Purpose | Rotation | Air-gap |
|---|---|---|---|---|---|
| `APP_ENV` | No | `development` | `development`/`staging`/`production` behavior switches | — | — |
| `PORT` | No | `8080` | API listen port | Restart | — |
| `WEBHOOKS_PORT` | No | `8081` | Webhooks gateway port | Restart | — |
| `APP_BASE_URL` | Yes (prod) | — | Public base URL; used for OAuth callbacks, SAML ACS/metadata, CLI loopback | Update IdP/OAuth registrations first | Must be internal URL |
| `SCANDRIX_CLOUD_MODE` / `API_CLOUD_MODE` | No | `false` | Cloud vs self-hosted behavior (domain-verifier strictness, SSO strictness, featuregate audience) | Restart | `false` |

## Data plane

| Variable | Required | Purpose | Rotation | Air-gap |
|---|---|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL (pgx pool). Supabase pooler URL supported (`sslmode=require`) | Rotate DB password → update secret → rolling restart (pool drains) | Must point at in-perimeter PG |
| `RABBITMQ_URL` | Yes | RabbitMQ (quorum queues, DLQ, delayed exchange) | Same as DB | In-perimeter broker |
| `REDIS_URL` | Yes | Redis (locks, idempotency, rate limit, cache) | Same as DB | In-perimeter Redis |
| `APPWRITE_ENDPOINT` / `APPWRITE_PROJECT_ID` / `APPWRITE_API_KEY` | Yes (artifacts) | Object storage for review artifacts | Rotate API key in Appwrite → update secret → restart | Replace with S3-compatible in-perimeter endpoint |
| `DOPPLER_TOKEN` / `DOPPLER_PROJECT` / `DOPPLER_CONFIG` | No | Secret-manager source (preferred in staging/prod over flat `.env`) | Per Doppler policy | Replace with customer vault |

## Auth & sessions

| Variable | Required | Purpose | Rotation |
|---|---|---|---|
| `JWT_SECRET` | Yes | HMAC for app JWTs. **Warning:** `cmd/server` also falls back to it as the SCIM bearer when `SCIM_BEARER_TOKEN` is unset — always set a dedicated SCIM token in production (removes the fallback). | Overlap: deploy verifiers accepting old+new (SPECCED procedure), then retire old. Until the overlap procedure lands, rotation = coordinated restart with brief session invalidation. |
| `SCIM_BEARER_TOKEN` | Yes (if SCIM used) | Dedicated SCIM 2.0 bearer. Constant-time compared. | Dual-accept window (SPECCED) or IdP maintenance window. |
| `KMS_MASTER_KEY` | Yes (BYOK) | Master key for AES-256-GCM envelope encryption of tenant keys | Re-encrypt tenant DEKs (runbook in `operations/rotation-runbooks.md`). |
| `INTEGRATION_ENCRYPTION_KEY` | Yes | Encrypts stored SCM/PM connection secrets | Same as KMS. |
| `SMTP_*` (HOST/PORT/USERNAME/PASSWORD/FROM) | Yes (email flows) | Transactional mail (verify, reset, dunning) | Provider-side, no overlap needed. |

## SCM & webhooks

| Variable | Required | Purpose |
|---|---|---|
| `GITHUB_TOKEN`, `GITLAB_*`, `BITBUCKET_*`, `AZURE_DEVOPS_PAT`, `FORGEJO_TOKEN`/`FORGEJO_BASE_URL` | Per connected provider | SCM API access for diff fetch + comment posting |
| `GITHUB_WEBHOOK_SECRET`, `GITLAB_WEBHOOK_SECRET`, `BITBUCKET_WEBHOOK_SECRET`, `AZURE_DEVOPS_WEBHOOK_SECRET`, `FORGEJO_WEBHOOK_SECRET` | Per webhook | HMAC verification — unverified payloads are dropped with 401, never processed |
| `GITHUB_OAUTH_CLIENT_ID/SECRET`, `GITLAB_OAUTH_*` (+ redirect URIs) | If OAuth login used | SCM OAuth; redirect URIs must match provider registration |

## LLM providers

Managed keys fund inference per tier (`CanAccessModel` gates); BYOK keys are per-workspace and bypass tier model lists. At least one funded path (managed key or workspace BYOK) is required or reviews fail closed with `BYOK_REQUIRED`.

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` | OpenAI or OpenAI-compatible endpoint (custom base URLs supported) |
| `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `DEEPSEEK_API_KEY` | Direct provider keys |
| `OPENROUTER_API_KEY` | OpenRouter aggregation |
| `BEDROCK_BEARER_TOKEN` / `BEDROCK_REGION`, `VERTEX_ACCESS_TOKEN`/`VERTEX_PROJECT_ID`/`VERTEX_LOCATION` | Cloud IAM-routed inference |
| `MOONSHOT_API_KEY`, `ALIBABA_API_KEY`, `MINIMAX_API_KEY`, `XAI_API_KEY`, `MISTRAL_API_KEY` | Direct provider keys |
| `AI_MODEL_DEFAULT/FALLBACK/TRIAGE/LOGIC/SECURITY/THREAT_MODEL/ARBITER/SYNTHESIZER` | Per-task model routing |
| `PROOFOFFIX_SANDBOX_ENABLED` | Sandbox verification toggle (`false` = findings post `unverified`, labeled) |

## Billing

| Variable | Required | Purpose |
|---|---|---|
| `RAZORPAY_KEY_ID` / `RAZORPAY_KEY_SECRET` | If billing enabled | Plan/subscription management |
| `RAZORPAY_WEBHOOK_SECRET` | If billing webhooks used | Webhook signature verification (unverified billing events are dropped) |

## Enterprise license (EE)

| Variable | Required | Purpose |
|---|---|---|
| `SCANDRIX_LICENSE_PUBLIC_KEY` | If licensed | Base64 Ed25519 vendor public key (PEM armour accepted). Absent = legitimate Community boot. Malformed = fail boot. |
| `SCANDRIX_LICENSE_KEY` | If licensed (xor file) | Signed envelope token. Invalid = fail boot (fail-closed). |
| `SCANDRIX_LICENSE_FILE` | If licensed (xor key) | Path to token file for secret-mount deployments. |
| `SCANDRIX_LICENSE_KEY_RING` | During key rotation | Comma-separated `keyID:base64PublicKey` pairs that stay valid alongside the primary key, so licenses signed by the outgoing key keep verifying until they expire. Malformed entry = fail boot (never silently skipped). |
| `SCANDRIX_HARDWARE_FINGERPRINT` | No | Expected hardware/cluster identity. When set, only licenses whose `hardware_fingerprint` matches load; an unbound license is rejected. Unset = no binding enforced. |
| `AUTHORITY_PRIVATE_KEY` | `scandrix-keygen` only | Authority signing key for minting licenses. **Secret** — HSM/secret manager only, never in source or images. Prefer `scandrix-keygen -keyfile` in automation. |

`scandrix-keygen` also reads the signing key from `-keyfile` (preferred) or `-privkey`, and writes a new keypair's private half to a `0600` file rather than stdout.

## Telemetry & hardening

| Variable | Purpose |
|---|---|
| `SENTRY_DSN` | Sentry on only when set; unset = fully inert (verified). |
| `SCANDRIX_TELEMETRY_DISABLED` | `true/1/yes` disables the self-hosted beacon (verified in transport). |
| `SCANDRIX_TELEMETRY_ENDPOINT` | Beacon destination override. |
| `SCANDRIX_VERSION` | Reported by beacon collector when set. |

## Maintenance rule

Adding a variable requires: code read site + `.env.example` name + a row here (purpose, rotation, air-gap) in the same PR. CI should fail on undocumented `os.Getenv` additions (SPECCED check).
