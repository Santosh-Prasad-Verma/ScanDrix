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

### 4. ScanDrix Enterprise Architecture & Engineering Suite
Complete engineering specification and visual architecture for the next-generation compiled Go enterprise platform:
- [Enterprise Product Requirements Document (PRD)](enterprise/PRD.md): Vision, personas, strategic differentiators, and functional epics.
- [Enterprise Technical Requirements Document (TRD)](enterprise/TRD.md): Go microservice topology, Tree-sitter AST, Ed25519 licensing, and partitioned warehouse.
- [Enterprise Visual Workflows & Sequence Architecture](enterprise/WORKFLOWS.md): Comprehensive Mermaid diagrams for multi-agent councils, sandbox runners, and air-gapped BYOK.
- [Enterprise Engineering Implementation Plan](enterprise/IMPLEMENTATION.md): Phased execution roadmap, interfaces, test matrix, and verification criteria.

### 5. Architecture (system anatomy)
- [System Architecture Overview](architecture/overview.md): binaries, ports, C4-style container map, request paths, sync/async rules.
- [Repository & Module Map](architecture/repository-map.md): all 13 `cmd/` + 51 `internal/` packages with anchor files, plus where new code goes.
- [Data Model](architecture/data-model.md): 47 tables, ERD, RLS contract, growth/retention status.
- [Event & Queue Catalog](architecture/event-catalog.md): exchanges/queues, payloads, outbox/inbox guarantees, debug order.
- [Deployment Topology](architecture/deployment-topology.md): SaaS/dedicated/self-hosted/air-gapped, ports, scaling units, failure domains.
- [Frontend Architecture](architecture/frontend-architecture.md): dashboard routes, auth/session flow, conventions, honest EE surface status.
- [Glossary](architecture/glossary.md): canonical terms + status vocabulary (IMPLEMENTED/STUB/SPECCED/TARGET).

### 6. Reference (contracts integrators build against)
- [Configuration Reference](reference/configuration.md): every environment variable — purpose, rotation impact, air-gap relevance.
- [API Reference](reference/api-reference.md): routes, auth, RBAC/feature gates, conventions. STUB/SPECCED rows are marked and must not be sold as working.

### 7. Evaluation (how quality numbers are earned)
- [Methodology](evaluation/methodology.md): golden corpus, frozen metric definitions, CI gates. No scorecard number ships without its corpus version attached.

### 8. Operations (how it runs at 3am)
- [Rotation Runbooks](operations/rotation-runbooks.md): one procedure per secret (REQ-8.3).
- [Backup, Restore & DR](operations/backup-restore.md): RPO/RTO per tier, seat-reconciliation rule (REQ-8.5).
- [Runbooks](operations/runbooks.md): queue drain, webhook floods, license expiry, compromise, failover, sandbox outage.
- [Upgrade & Migration](operations/upgrade-migration.md): numbering policy, compat matrix, release checklist.
- [SLOs & Error Budgets](operations/slo-error-budgets.md): PRD §5 as alerts, paging, and budget policy.

### 9. Least-Privilege Rollout (current infrastructure security state)
- [Status & Remaining Work](LEAST_PRIVILEGE_ROLLOUT.md): verified vs unverified, the two RLS contexts every query must declare, the 5 remaining items with requirements and implementation, credentials checklist, rollback.

### 10. Security (how it is certified and sold)
- [Threat Model](security/threat-model.md): per-boundary threats, mitigations with code pointers, residuals.
- [Security Whitepaper](security/security-whitepaper.md): customer-facing, every claim evidenced; alignment vs certification never interchanged.
- [Egress Allowlist & Audit](security/egress-allowlist.md): default-deny table per mode + quarterly audit (PRD §6 gate).
- [DPA Support Pack](security/dpa-support.md): processing summary, residency, breach SLA, subject rights, audit rights.

### 11. Decisions (why things are the way they are)
- [ADR-0001](decisions/ADR-0001-grace-period.md): 7-day grace. [ADR-0002](decisions/ADR-0002-license-envelope.md): envelope format. [ADR-0003](decisions/ADR-0003-parser-build.md): CGO-vs-pure-Go fork (open). [ADR-0004](decisions/ADR-0004-canonical-rbac.md): single RBAC matrix. [ADR-0005](decisions/ADR-0005-no-parallel-trees.md): extend, don't duplicate.

> Honesty rule for the whole tree: IMPLEMENTED / STUB / SPECCED / TARGET labels are normative. A doc that presents a SPECCED behavior as working is a defect — file it like one.

