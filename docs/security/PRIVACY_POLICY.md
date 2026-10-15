# ScanDrix Privacy Policy

**Effective Date:** 2026-10-15  
**Version:** 1.0.0  
**Contact:** `privacy@scandrix.dev`  

---

## 1. Overview & Scope

ScanDrix AI Inc. ("ScanDrix", "we", "us", or "our") provides an AI-powered code review and static security analysis platform. This Privacy Policy describes how we collect, process, store, and protect information when you visit `scandrix.dev`, use the ScanDrix Dashboard, or integrate our services with your source code management (SCM) platforms.

---

## 2. Information We Collect

### 2.1 Account & Identity Information
When you register an account or sign in via OAuth (GitHub, GitLab, Bitbucket, SSO):
* Name, email address, SCM username, avatar URL, and organization membership.
* Authentication tokens and device IDs for CLI sessions.

### 2.2 Customer Code & Review Metadata
* **Git Diffs**: Unified diffs and surrounding context lines of code submitted in pull requests.
* **Review Outputs**: Review comments, security findings, severity ratings, and automated quick-fix diffs.
* **Telemetry & Metrics**: Pull request review durations, finding counts, and rule match statistics.

> [!IMPORTANT]
> **Zero Model Training**: ScanDrix does **NOT** use customer proprietary source code, diffs, comments, or repository metadata to train foundational artificial intelligence models.

---

## 3. How We Use Information

We process collected data solely to:
1. Provide, execute, and maintain automated code reviews and security audits.
2. Verify authorization and enforce Row-Level Security multi-tenant isolation.
3. Facilitate billing, seat quota management, and license validation.
4. Detect security incidents, prevent abuse, and investigate system anomalies.

---

## 4. Subprocessors & Third-Party Sharing

ScanDrix does not sell customer personal information or source code. We engage third-party subprocessors under strict Data Processing Addenda (DPAs) with confidentiality and data protection obligations:
* **Cloud Infrastructure**: AWS (Compute, RDS PostgreSQL, S3, Secrets Manager).
* **AI Model Providers**: Anthropic, OpenAI, and Google Cloud (operating under Zero Data Retention agreements).
* **Observability & Error Tracking**: Sentry, BetterStack.
* **Transactional Communication**: Zoho Mail / Bookings.

See our full [Subprocessors List](file:///home/tarun/Videos/kodus-ai/ScanDrix/docs/security/SUBPROCESSORS.md) for details.

---

## 5. Data Retention & Deletion Rights

* **Review History**: Retained for the duration of the customer's active subscription.
* **Zero Data Retention (ZDR) Option**: Organizations can enable Zero Data Retention in Organization Parameters to immediately purge code snippets upon review completion.
* **Account & Repository Deletion**: When a repository is disconnected or an account is deleted, all associated workspace data is permanently purged in accordance with our retention policy.

---

## 6. Security Standards

* **Encryption**: TLS 1.3 in transit; AES-256-GCM envelope encryption at rest for all stored credentials and API keys.
* **Access Control**: PostgreSQL Row-Level Security enforced across all tenant records.

---

## 7. Contact Information

For privacy questions, GDPR data subject requests, or DPA inquiries:
* **Email**: `privacy@scandrix.dev`
* **Security & Vulnerability Reports**: `security@scandrix.dev`
