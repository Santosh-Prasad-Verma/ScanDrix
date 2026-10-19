# ScanDrix AI Data Boundaries & Model Privacy Policy

**Effective Date:** 2026-10-15  
**Version:** 1.0.0  
**Target Audience:** Enterprise Customers, Security Officers, Compliance Auditors  

---

## 1. Executive Summary & Zero-Training Commitment

ScanDrix is an AI-powered code review and static security analysis platform. Protecting customer intellectual property and proprietary source code is our highest operational priority.

> [!IMPORTANT]
> **Zero Model Training Guarantee**  
> ScanDrix does **NOT** use customer source code, pull request diffs, review comments, file trees, or repository metadata to train, retrain, fine-tune, or evaluate any artificial intelligence or machine learning models—neither our own nor third-party models.

---

## 2. Ingestion Boundaries: What Code Leaves the Customer Boundary

ScanDrix strictly limits data ingestion to the minimum context required to perform code reviews.

### 2.1 Ingested Data
* **Unified Git Diffs**: Lines added, modified, or deleted within the target pull request or commit.
* **Bounded Context Windows**: Surrounding lines of code directly adjacent to the change, imported interfaces, and syntax trees necessary to verify type safety and security context.
* **Configuration & Rules**: Custom Drixy review rules, `.drixy/rules.yaml`, and organization review parameters.

### 2.2 Excluded Data (Never Ingested or Sent to AI)
* **Full Repository Archives**: ScanDrix never clones or uploads entire repositories to AI model providers.
* **Secrets & Environment Variables**: Files matching `.env*`, `*.pem`, `*.key`, `credentials.json`, or high-entropy tokens detected by our pre-flight secret scanner are stripped prior to analysis.
* **Out-of-Scope Files**: Unchanged files, build artifacts (`node_modules/`, `target/`, `dist/`), binary files, and documentation assets are omitted.

---

## 3. Supported AI Inference Models & Retention Terms

ScanDrix provides three distinct inference deployment modes to accommodate varying organizational compliance requirements:

| Inference Mode | Description | Model Providers | Data Retention Policy |
|---|---|---|---|
| **ScanDrix Managed AI (Default)** | ScanDrix routes review prompts through enterprise-grade API endpoints. | Anthropic (Claude 3.7/Sonnet), OpenAI (GPT-4o), Google Gemini 2.0 | **Zero Data Retention (ZDR)**: Prompts and completions are processed in volatile memory and are **not** persisted or logged by provider. |
| **Bring Your Own Key (BYOK)** | Customer supplies their own direct API credentials via AES-256-GCM encrypted parameters. | Anthropic, OpenAI, Gemini, Bedrock, Vertex AI, Novita | Governed directly by customer's own commercial Data Processing Addendum (DPA) and enterprise terms with the provider. |
| **Self-Hosted / Local (Air-Gapped)** | Inference executes entirely within customer's VPC or bare-metal infrastructure. | Ollama, vLLM, Local OpenAI-compatible server | **Zero external transmission**: Exactly 0 bytes leave customer's internal network perimeter. |

---

## 4. Storage & Persistence Lifecycle (REQ-8.2)

### 4.1 Transient Processing Pipeline
1. **Fetch & Isolate**: Git diff is extracted inside an isolated execution sandbox.
2. **Pre-flight Filter**: Secret scanner strips high-entropy tokens and environment files.
3. **Inference**: Diff and active review rules are dispatched over TLS 1.3 to the configured model.
4. **Structured Output**: AI response is validated against JSON schema (generating findings, line numbers, and suggestions).
5. **Purge**: Volatile review buffers in memory are wiped.

### 4.2 Zero Data Retention (ZDR) Mode
When **Zero Data Retention** is toggled in Organization Parameters:
* Only aggregated metrics (finding counts, latency, rule execution status) and cryptographic SHA-256 hashes of reviews are retained in PostgreSQL.
* Transient code snippets, review diff caches, and source excerpts are purged automatically upon review delivery.

---

## 5. Security & Encryption Standards

* **In Transit**: All outbound communication with AI provider APIs enforces TLS 1.3 with forward secrecy. Custom endpoints must enforce HTTPS with socket-level IP pinning (`AssertSafeEndpoint` / `AssertSafeURL`) to prevent SSRF.
* **At Rest**: BYOK provider credentials and team tokens are encrypted using AES-256-GCM envelope encryption via `SCANDRIX_ENCRYPTION_KEY`.
* **Access Control**: PostgreSQL Row-Level Security (`FORCE ROW LEVEL SECURITY`) strictly isolates all review records by tenant workspace ID (`app.current_workspace_id`).

---

## 6. Audit & Verification

Enterprise customers can verify model interaction boundaries through:
* **Tamper-Evident Audit Logs**: Queryable via `internal/enterprise/audit/events.go` recording provider, model ID, timestamp, and operator without storing raw source code.
* **Egress Firewall Rules**: Listed in [`docs/security/egress-allowlist.md`](./egress-allowlist.md) for strict network gateway enforcement.

For security inquiries or specific DPA signing, contact **`security@scandrix.dev`**.
