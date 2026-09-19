# 📚 ScanDrix Documentation

Welcome to the technical documentation repository for **ScanDrix**, an enterprise-grade AI-powered code review and autonomous security assurance platform.

---

## 🗂️ Documentation Sections

### 1. [ScanDrix CLI Manual](cli/README.md)
Comprehensive reference guide covering the Go-native ScanDrix CLI binary:
- Quick Start & Installation
- Command Reference & Flag Specifications
- Agent Envelopes (Schema v1.0) & Field Masking (`--fields`)
- Git Hooks Guard (`scandrix hook install`)
- Custom Rules & Local AST Evaluation (`scandrix rules validate`)
- Native Superpowers (`chat`, `tui`, `scan`, `fix`, `export`, `pentest`, `mcp`, `server`, `trace`)

### 2. [CLI Architectural Implementation Plans & QA Specs](cli/plans/)
Detailed execution plans, architectural extraction design documents, and manual QA testing checklists:
- [`2026-03-10-config-repo-command.md`](cli/plans/2026-03-10-config-repo-command.md): Repository tracking and registration command design.
- [`2026-03-12-repo-config-cli-manual-qa.md`](cli/plans/2026-03-12-repo-config-cli-manual-qa.md): Manual terminal QA testing matrix and smoke testing checklist.
- [`2026-03-12-repo-config-cli-onboarding.md`](cli/plans/2026-03-12-repo-config-cli-onboarding.md): Developer onboarding and interactive setup wizard specification.
- [`2026-03-12-repo-settings-web-api-alignment.md`](cli/plans/2026-03-12-repo-settings-web-api-alignment.md): Repository settings API schema alignment.
- [`2026-03-13-api-auth-module-extraction.md`](cli/plans/2026-03-13-api-auth-module-extraction.md): Modular API client auth service separation.
- [`2026-03-13-api-config-module-extraction.md`](cli/plans/2026-03-13-api-config-module-extraction.md): Modular API client config service separation.
- [`2026-03-13-api-review-module-extraction.md`](cli/plans/2026-03-13-api-review-module-extraction.md): Modular API client review service separation.
- [`2026-03-13-api-trial-memory-module-extraction.md`](cli/plans/2026-03-13-api-trial-memory-module-extraction.md): Modular trial and decision memory capture client.
- [`2026-03-13-repo-config-module-extraction.md`](cli/plans/2026-03-13-repo-config-module-extraction.md): Repository config domain extraction.
- [`2026-03-13-repo-config-review-followups.md`](cli/plans/2026-03-13-repo-config-review-followups.md): Post-review follow-ups and error normalization.
- [`2026-03-13-repo-config-wizard-ux.md`](cli/plans/2026-03-13-repo-config-wizard-ux.md): Interactive terminal prompt and configuration UX.
- [`2026-03-13-types-domain-extraction.md`](cli/plans/2026-03-13-types-domain-extraction.md): Domain types and interface isolation.

### 3. [ScanDrix Enterprise Authentication Architecture & Workflows](AUTHENTICATION_WORKFLOWS.md)
Comprehensive zero-trust enterprise authentication reference with Mermaid sequence diagrams, state machines, and flowcharts:
- Web User Session & Argon2id + JWT Lifecycle
- SCM OAuth 2.0 (GitHub & GitLab)
- CLI Local Loopback & RFC 8628 Device Authorization Flows
- Enterprise SAML 2.0 / OIDC Single Sign-On & DNS TXT Domain Verification
- SSO Diagnostic Test Workbench Sandbox
- Team CLI Keys (`scandrix_*`) & Hardware Quota Limits
- Fine-Grained Per-User Repository Boundaries (RBAC)
- SCIM 2.0 Automated User Provisioning & Deprovisioning
- Background Orphan Session Sweepers & OWASP ASVS v4.0.3 Compliance Matrix
