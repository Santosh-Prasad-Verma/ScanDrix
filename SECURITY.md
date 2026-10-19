# Security Policy

## Reporting a vulnerability

**Do not open a public issue, pull request, or discussion for a security bug.**

Report privately to **security@scandrix.dev** — the same address published in
[`docs/security/security.txt`](docs/security/security.txt) and served at
`https://scandrix.dev/.well-known/security.txt`.

Please include:

- the affected component (service, binary, or route),
- the steps to reproduce, and
- the impact you observed.

You will get an acknowledgement within 3 business days. If the report is
valid, you will get a severity assessment and a remediation plan.

### What to expect

Severity follows the classification already used in the incident runbook,
[`docs/security/INCIDENT_RESPONSE.md`](docs/security/INCIDENT_RESPONSE.md):

| Severity | Definition | Containment target |
|---|---|---|
| SEV-0 (Critical) | Active compromise of customer source code, RLS tenant-isolation failure, exposure of `SCANDRIX_ENCRYPTION_KEY`, or remote code execution | 30 minutes |
| SEV-1 (High) | Broken authentication, privilege escalation, CSRF on mutating endpoints, SSRF with internal metadata access | 2 hours |
| SEV-2 (Medium) | DoS on webhook ingestion, rate-limit bypass, transient worker anomalies | 8 hours |
| SEV-3 (Low) | Missing security headers, low-impact advisories with no reachable path | Next sprint |

### Scope

In scope:

- the Go engine and its services in this repository,
- the ScanDrix CLI distributed from this repository,
- the migrations and infrastructure-as-code under `infra/`,
- the deployed multi-tenant behaviour (authentication, authorization, RLS).

Out of scope:

- findings that require an already-compromised host or an already-leaked
  credential,
- findings from automated scanners with no demonstrated path to impact and no
  proof-of-concept,
- reports about the availability of a self-hosted deployment that the
  operator has not configured.

The frontends are separate repositories with their own reporting routes.

## Supported versions

Only the current release line receives security fixes. Fixes are delivered
through a new patch release on the `v*.*.*` tag series; there is no
long-term-support branch.

| Version | Supported |
|---|---|
| Latest `v*.*.*` release | Yes |
| Any earlier tag | No — upgrade to the latest release |

## Reporting a credential exposure

If a secret is committed to this repository, treat it as compromised and
rotate it at the provider. Deleting the line does not remove it from git
history, and the object stays reachable from any existing clone, reflog, or
fork.

The repository enforces this automatically: `.github/workflows/secret-scan.yml`
runs [Gitleaks](https://github.com/gitleaks/gitleaks) over the full history on
a schedule and over the diff on every push, and a second job asserts that no
credential-bearing file is tracked and that `.env.example` carries names
only, never real values.

## Security controls in this repository

- **Secrets.** Every credential is read from the environment. `.env`, `.env.*`
  and Terraform state are git-ignored; `.env.example` and
  `.env.production.template` carry variable names and empty placeholders only.
- **Static analysis.** `gosec` and `govulncheck` run in
  `.github/workflows/security-audit.yml`; `golangci-lint` runs in
  `.github/workflows/golangci-lint.yml`.
- **Container scanning.** Trivy scans every service image in
  `.github/workflows/docker-build-scan.yml` before it can be published.
- **Tenant isolation.** Row-level security is enforced in the database, and the
  test suite provisions a least-privilege runtime role specifically so the
  RLS tests cannot pass vacuously against a superuser.
- **Least privilege.** CI workflows declare `permissions:` explicitly;
  Terraform plan runs only from a trusted ref, never from fork-controlled
  configuration.

## Related documents

- [`docs/security/INCIDENT_RESPONSE.md`](docs/security/INCIDENT_RESPONSE.md) —
  severity classification, token and session revocation runbook
- [`docs/security/threat-model.md`](docs/security/threat-model.md)
- [`docs/security/AI_DATA_BOUNDARIES.md`](docs/security/AI_DATA_BOUNDARIES.md)
- [`docs/security/egress-allowlist.md`](docs/security/egress-allowlist.md)
- [`docs/security/PRIVACY_POLICY.md`](docs/security/PRIVACY_POLICY.md) —
  privacy policy
- [`docs/security/security-whitepaper.md`](docs/security/security-whitepaper.md)
- [`docs/security/SUBPROCESSORS.md`](docs/security/SUBPROCESSORS.md)
