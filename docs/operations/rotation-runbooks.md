# Secret Rotation Runbooks (REQ-8.3)

Every secret has exactly one source and one procedure here. Triggers: scheduled (interval per secret), on personnel change, or on suspected compromise (then follow `runbooks.md` §compromise first, rotate second).

## 1. License authority keypair (highest blast radius)

- **Source:** HSM/vault only. The private half never touches developer machines, CI logs, or chat.
- **Overlap procedure (zero downtime):**
  1. Generate new pair offline; record new `key_id` (date-based, e.g. `2026-10`).
  2. Deploy new public key to all servers alongside the old (config change, rolling restart). Old licenses still verify.
  3. Mint renewed licenses with `key_id` set; distribute to customers with the overlap notice.
  4. After every active old-key license has expired or been re-issued (overlap window ≥ longest outstanding `ExpiresAt`), remove the old public key.
- **Status:** **[IMPLEMENTED]** in `internal/enterprise/license/validator.go`. Overlap-capable verifiers accept old+new during the rotation window via `SCANDRIX_LICENSE_KEY_RING`. Zero-downtime rolling key cutovers are fully supported.

## 2. `JWT_SECRET`

- **Source:** secret manager (never flat `.env` in prod).
- **Procedure:** overlap-capable verifiers accept old+new during the window (SPECCED); until then, rotate during low-traffic with announced session invalidation (refresh tokens issued under the old secret stop verifying).
- **Interval:** 90 days or on compromise.

## 3. `SCANDRIX_LICENSE_PUBLIC_KEY` (server-side copy)

- Content change only (no format change); rolling restart. Mismatched key = fail boot by design — verify with `scandrix-keygen verify` (Phase 1 tooling) against a staging node before fleet rollout.

## 4. `SCIM_BEARER_TOKEN`

- **Procedure:** **[IMPLEMENTED]** via `scim_tenant_tokens` (migration 037). Workspace SCIM tokens can be rotated dynamically with overlap without service restarts. IdP provisioner config is updated to the new token, and old tokens are invalidated upon confirmation.
- **Always set a dedicated token** — the `JWT_SECRET` fallback in `cmd/server` exists for dev only and must be removed from production configs (audit flag).

## 5. Webhook secrets (Git providers + Razorpay)

- Rotate per-provider: generate in provider dashboard → update secret → send test webhook → confirm 200 + processed (not 401) → retire old where the provider supports dual secrets; otherwise accept a minutes-long ingestion gap and announce it.

## 6. `KMS_MASTER_KEY` / `INTEGRATION_ENCRYPTION_KEY`

- Re-encrypt tenant DEKs/secrets under the new key (envelope re-wrap job), verify decrypt Rot spot-checks per workspace, then retire. Full backup before starting (see `backup-restore.md`).

## 7. SCM tokens / OAuth client secrets / SMTP credentials

- Provider-side rotation; update secret; restart affected service; verify with a test diff-fetch (SCM) or test mail (SMTP). No overlap needed (single-writer).

## Record

Every rotation logs: date, operator, secret, old-key retirement confirmation. The log itself contains no secret material — only key IDs and timestamps.
