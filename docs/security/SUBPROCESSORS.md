# ScanDrix Subprocessors Registry

**Effective Date:** 2026-10-15  
**Version:** 1.0.0  
**Contact:** `privacy@scandrix.dev`  

---

## 1. Overview

To provide our cloud-hosted AI code review platform, ScanDrix engages third-party service providers ("Subprocessors"). Each subprocessor undergoes rigorous security evaluations, complies with applicable data protection regulations (including GDPR and CCPA), and operates under written Data Processing Agreements (DPAs).

---

## 2. Infrastructure & Hosting Subprocessors

| Subprocessor | Purpose | Entity Location | Data Processed |
|---|---|---|---|
| **Amazon Web Services (AWS)** | Cloud compute (ECS/EC2), relational database (PostgreSQL/RDS), object storage (S3), and secret management. | United States / EU (region-selectable) | Customer account data, encrypted credentials, review metadata, outbox queue events. |
| **Cloudflare Inc.** | Global DNS, DDoS protection, CDN, and SSL/TLS termination. | United States / Global Edge | Transient network traffic, request IP addresses, TLS session data. |

---

## 3. AI & LLM Inference Subprocessors

> [!NOTE]
> All model inference providers utilized by ScanDrix Managed AI operate under **Zero Data Retention (ZDR)** agreements. Prompts and completions are processed in memory and are **never** used to train models.

| Subprocessor | Purpose | Entity Location | Data Processed |
|---|---|---|---|
| **Anthropic PBC** | AI code review and reasoning engine (Claude 3.7 / Sonnet). | United States | Code diffs and review rule prompts (transient in-memory processing). |
| **OpenAI LLC** | AI code review and code repair synthesis (GPT-4o). | United States | Code diffs and review rule prompts (transient in-memory processing). |
| **Google Cloud (Gemini / Vertex AI)** | Multimodal and large context-window review processing. | United States | Code diffs and review rule prompts (transient in-memory processing). |

*Note: For customers utilizing **Bring Your Own Key (BYOK)** or self-hosted local models (e.g., Ollama, vLLM), inference data routes directly under the customer's own agreements or stays entirely inside their private network.*

---

## 4. Observability & Customer Support Subprocessors

| Subprocessor | Purpose | Entity Location | Data Processed |
|---|---|---|---|
| **Functional Software Inc. (Sentry)** | Application exception tracking and performance diagnostics. | United States | System stack traces, error messages (code and secrets stripped). |
| **BetterStack Inc.** | Heartbeat health monitoring and infrastructure uptime status. | United States / EU | Service availability pings and synthetic endpoint metrics. |
| **Zoho Corporation** | Corporate email, customer support, and calendar bookings. | India / Global | Customer contact details, support requests, and scheduled demos. |

---

## 5. Updates & Notification of New Subprocessors

ScanDrix maintains an active notification mechanism for subprocessor changes. Customers may subscribe to subprocessor notifications by emailing **`privacy@scandrix.dev`**. Customers have 30 days to object to a new subprocessor on reasonable data protection grounds.
