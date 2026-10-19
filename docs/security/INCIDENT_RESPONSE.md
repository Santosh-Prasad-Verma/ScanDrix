# ScanDrix Security Incident Response & Token Revocation Runbook

**Document Owner:** ScanDrix Security & Reliability Engineering  
**Classification:** Internal & Compliance Reference  
**Effective Date:** 2026-10-15  
**Version:** 1.0.0  

---

## 1. Incident Severity Classification

| Severity | Definition | Target Containment | Customer Notice SLA |
|---|---|---|---|
| **SEV-0 (Critical)** | Active compromise of customer source code, RLS tenant isolation failure, exposure of master encryption key (`SCANDRIX_ENCRYPTION_KEY`), or remote code execution. | ≤ 30 minutes | ≤ 24 hours |
| **SEV-1 (High)** | Broken authentication, unauthorized privilege escalation, cross-site request forgery on mutating endpoints, or SSRF with internal metadata access. | ≤ 2 hours | ≤ 72 hours |
| **SEV-2 (Medium)** | Distributed denial of service on webhook ingestion, bypass of rate limits, or transient worker processing anomalies. | ≤ 8 hours | Release notes / Advisory |
| **SEV-3 (Low)** | Minor security header omissions, low-impact dependency advisories without direct exploitable paths. | Next sprint | Scheduled release |

---

## 2. Emergency Containment: Instant Token & Session Revocation

When an account or credential is suspected of compromise, containment must precede investigation.

### 2.1 Instant User Session Revocation
ScanDrix maintains an active revocation table in PostgreSQL (`revoked_access_tokens`, migration 041). Any token issued prior to the recorded epoch cutoff is rejected immediately by all API instances.

To instantly revoke all active JWT tokens for a compromised user:
```sql
-- Replace $USER_ID with the compromised user's UUID
INSERT INTO revoked_access_tokens (user_id, issued_at, expires_at)
VALUES (
    '$USER_ID',
    EXTRACT(EPOCH FROM now())::bigint,
    now() + interval '24 hours'
)
ON CONFLICT (user_id, issued_at) 
DO UPDATE SET expires_at = GREATEST(revoked_access_tokens.expires_at, EXCLUDED.expires_at);
```

### 2.2 Instant Team CLI Key Revocation
To deactivate a leaked Team API Key (`scandrix_*`):
```sql
UPDATE team_cli_key 
SET is_active = false, 
    revoked_at = now() 
WHERE id = '$TEAM_KEY_ID' OR key_hash = encode(sha256('$LEAKED_KEY'::bytea), 'hex');
```

### 2.3 Quarantine / Freeze an Entire Tenant Workspace
If cross-tenant probe attempts or malicious automated reviews are detected:
```sql
UPDATE workspaces 
SET is_active = false 
WHERE id = '$WORKSPACE_ID';
```

---

## 3. Credential & Secret Rotation Procedures

All rotation procedures must strictly follow [`docs/operations/rotation-runbooks.md`](../operations/rotation-runbooks.md):

1. **`JWT_SECRET` Rotation**:
   * Generate new 256-bit secret: `openssl rand -hex 32`.
   * Update secret in AWS Secrets Manager / environment.
   * Trigger rolling restart of API services. All existing refresh tokens will require re-login.
2. **`SCANDRIX_ENCRYPTION_KEY` Rotation**:
   * Execute key envelope re-wrap job to re-encrypt all stored BYOK credentials under the new AES-256-GCM key before retiring the old key.
3. **GitHub App Private Key Compromise**:
   * Generate a secondary private key in the GitHub App settings.
   * Update `GITHUB_APP_PRIVATE_KEY` in production secrets manager.
   * Verify token minting with `internal/platform/github/token_rotator.go`.
   * Revoke the compromised private key in GitHub App console.
4. **SCM & Razorpay Webhook Secrets**:
   * Generate replacement high-entropy secret.
   * Update target provider dashboard webhook secret.
   * Update corresponding `WEBHOOK_SECRET` environment variables.

---

## 4. Forensic Investigation & Evidence Gathering

1. **Tamper-Evident Audit Log Verification**:
   Query immutable audit records emitted by `internal/enterprise/audit/events.go`:
   ```sql
   SELECT id, workspace_id, actor_id, event_type, ip_address, user_agent, created_at
   FROM audit_events
   WHERE workspace_id = '$WORKSPACE_ID'
     AND created_at >= now() - interval '7 days'
   ORDER BY created_at DESC;
   ```
2. **Worker Sandbox Isolation Check**:
   Inspect active sandbox leases in `sandbox_leases` table to ensure untrusted code did not escape the isolated execution boundary.

---

## 5. Customer Notification & Breach Disclosure Plan

Under GDPR and our enterprise customer SLA commitments:

1. **Mandatory Notification Window**: Customers must be notified within **72 hours** of confirmed unauthorized access to customer data.
2. **Customer Advisory Structure**:
   * **Incident Summary**: Nature and duration of the event.
   * **Impacted Scope**: Specific repositories, pull requests, or user accounts affected.
   * **Technical Root Cause**: Explanation of the vulnerability or compromised credential.
   * **Remediation Completed**: Actions taken by ScanDrix (revocation, patching, rotation).
   * **Recommended Customer Actions**: Customer-side actions (rotating SCM access tokens, revoking personal API keys).
3. **Escalation & Communication Channel**:
   * Official advisories are dispatched from **`security@scandrix.dev`** directly to registered organization security administrators.
