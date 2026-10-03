# Security Whitepaper (customer-facing, every claim evidenced)

## 1. Architecture summary

ScanDrix deploys as Go services (API, webhooks gateway, workers, optional monolith `scandrix-server`) over PostgreSQL (row-level tenant isolation), Redis (locks/quotas), RabbitMQ quorum queues (durable review jobs + DLQ), and S3-compatible object storage for artifacts. See `enterprise/TRD.md` §1 for topology.

## 2. Shared responsibility

| Layer | ScanDrix (SaaS) | Customer (self-hosted/air-gapped) |
|---|---|---|
| Application hardening, RLS, secret handling | ✅ | ✅ (same binaries) |
| Infrastructure patching, network controls | ✅ | Customer (hardening guide on request) |
| Secret values (DB passwords, provider keys) | Customer-provided, never logged | Same |
| Backups meeting RPO/RTO | ✅ per tier (`operations/backup-restore.md`) | Customer executes with our runbooks |
| Egress compliance | ✅ SaaS allowlist | Customer firewall + our quarterly audit procedure |

## 3. Encryption inventory (verified, not asserted)

| Data | In transit | At rest |
|---|---|---|
| App ↔ PG/Redis/RabbitMQ | TLS (require `sslmode=require` / TLS ports; verified in config) | Provider-managed disk encryption + PG backups encrypted |
| Customer code in transit (diff fetch, comments) | HTTPS to SCM APIs | — (transient) |
| Stored snippets/artifacts | HTTPS | Object-store SSE; retention-purged per `REQ-8.2` schedule once the purger lands (SPECCED) |
| BYOK keys, SCM secrets | Never in URLs/logs | AES-256-GCM envelope (`INTEGRATION_ENCRYPTION_KEY` / `KMS_MASTER_KEY`) |
| Sessions/JWT | `Secure`/`httpOnly`/`SameSite` cookies (dashboard `httpOnly:false` is a tracked fix, not the standard) | Hashes only; refresh tokens revocable |

## 4. Identity & access

SAML 2.0 (X.509-verified) + OIDC + SCIM 2.0 with deprovisioning that revokes sessions; canonical RBAC (`OWNER/ADMIN/MEMBER/VIEWER`) with per-repo allowlists; MFA TOTP enforceable per workspace (SPECCED, REQ-5.4). brute-force backs off; admin actions are audit-logged.

## 5. Audit & compliance posture

Hash-chained tamper-evident audit log with CEF/syslog/JSON export; trusted-timestamp mechanism per REQ-8.1 (RFC 3161 **or** documented equivalent — the active mechanism is named in the release notes, never left ambiguous). SOC 2 Type II / ISO 27001 **alignment** is claimed; **certification** is claimed only with certificate number and date. The two words are never interchanged in our material.

## 6. Data handling & retention

Code is processed to produce reviews; snippet persistence follows the retention schedule (default + zero-retention mode with metrics+hashes only, enforced by purger per REQ-8.2). Subprocessors and regions are listed per deployment in the order form — no global list is asserted here because it varies by tier/mode.

## 7. Vulnerability management

- Coordinated disclosure: `security.txt` contact, 90-day default embargo, severity-rated fix SLAs (Critical 7d / High 30d / Medium 90d).
- `gosec` + dependency audit in CI; `go-licenses` commercial-binary gate; signed release artifacts (SPECCED).
- Pen-test: annual third-party test; summary available under NDA. A test that hasn't happened is never implied.
