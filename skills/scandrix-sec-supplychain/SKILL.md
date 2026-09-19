---
name: scandrix-sec-supplychain
description: Dependency management, SBOM verification, and pipeline integrity
---

# ScanDrix Supply Chain Guard

### Dependency Hygiene
- **Deterministic Builds**: Commit lockfiles (`go.sum`, `package-lock.json`, `pnpm-lock.yaml`, `Cargo.lock`, `poetry.lock`).
- **Audit Integrations**: Mandate vulnerability scanning checks (`govulncheck`, `npm audit --audit-level=high`, `trivy fs .`, `pip-audit`).
- **Minimal Surface**: Flag and review any newly introduced external dependencies that can be implemented cleanly with standard libraries.
- **Typosquatting Checks**: Verify package namespace and download counts for third-party libraries.
