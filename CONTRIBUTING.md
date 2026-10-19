# Contributing to ScanDrix

ScanDrix is a proprietary, enterprise-licensed codebase. The
[`LICENSE`](LICENSE) governs use of the software; this document describes how
to contribute to it.

## Repository layout

This repository holds the Go engine only. The frontends are separate
repositories and are not vendored here:

| Repository | Contents |
|---|---|
| `ScanDrix` (this repo) | Go services, CLI, migrations, Terraform, CI/CD |
| `ScanDrix-Dashboard` | Next.js dashboard + BFF proxy |
| `ScanDrix-website` | Next.js marketing site and docs |

A cross-cutting change usually needs a PR in more than one of them. Open them
in dependency order — the backend first, so the frontends can consume the new
contract.

## Prerequisites

- Go 1.25 or later (see `go.mod`)
- Docker with Compose (Postgres 16 + pgvector, RabbitMQ) for the integration
  and end-to-end tests
- `golangci-lint` for the lint gate
- `node` only for the Bruno contract tests (`make test-bruno`)

## Build, test, lint

All commands run from the repository root.

```bash
make build        # compile every binary into ./bin
make test         # unit and integration tests
make test-race    # same suite under the race detector
make lint         # go vet
make env-check    # validate the local environment against the contract
make docker-up    # bring up Postgres + RabbitMQ for the database-backed tests
make docker-down
```

The database-backed security tests are skipped — not failed — when no database
is reachable. `make docker-up` before `make test` if you want them to run.

## Branch and commit conventions

Branch names follow the pattern already in use in this repository:

```
feat/<short-description>
fix/<short-description>
chore/<short-description>
ci/<short-description>
remediation/<ticket-or-audit-id>
```

Commit messages use Conventional Commits with a scope:

```
feat(audit): remediate production readiness blockers
fix(security): address audit findings F01-F03
ci(workflows): graceful EC2 offline handling
chore(config): set default APP_BASE_URL
```

The scope names the area touched (`api`, `auth`, `security`, `ci`,
`notifications`, `billing`, `config`, `site`, `skills`). A subject line
starting with `feat`, `fix`, `ci`, `chore`, `perf`, `docs`, `refactor`, `test`
or `revert` is what the release notes are generated from.

## Pull requests

1. Branch from `main`.
2. Keep the change focused. Unrelated refactors in the same PR make the
   security review harder, not easier.
3. Make sure the local gates pass:
   ```bash
   make build && make test && make lint
   ```
4. Open a PR against `main` and describe what changed and why. If the change
   touches authentication, authorization, tenant isolation, billing, or the
   data model, say so explicitly in the description — those areas get a
   different review depth.
5. If the change adds or alters a database table, add the migration under
   `migrations/` in the same PR. The migration must be idempotent and must
   not drop or rewrite a column that already holds production data.

## What CI enforces

Every pull request runs these, and a red one blocks the merge:

| Workflow | Gate |
|---|---|
| `pr-quality-gate.yml` | `go mod verify`, `go mod tidy` clean, migrations applied, race-detector test suite with coverage, all binaries compile |
| `ci.yml` | Lint + unit tests with a real Postgres service |
| `golangci-lint.yml` | `golangci-lint` static analysis |
| `secret-scan.yml` | Gitleaks over the history, plus an assertion that no credential-bearing file is tracked |
| `security-audit.yml` | `govulncheck` and `gosec` SAST |
| `docker-build-scan.yml` | Every service image builds and passes a Trivy scan |
| `e2e-matrix.yml` | Postgres and RabbitMQ integration matrix |
| `terraform-plan.yml` | `fmt` and `validate` on untrusted PRs; `plan` only from a trusted ref |

`secret-scan.yml` is not optional and not advisory. If it fails on a finding
you believe is a false positive, add an allowlist entry in
[`.gitleaks.toml`](.gitleaks.toml) with a comment explaining why — never
allowlist a real credential to make the scan go green.

## Secrets

Every credential is read from the environment. Never commit a real value.

- Copy `.env.example` to `.env` for local development. It carries variable
  names and empty placeholders only.
- `.env`, `.env.*`, `*.pem`, `*.key`, `*.p12`, Terraform state and
  `*.tfvars` are git-ignored. Do not add exceptions for them.
- If a secret is committed, rotate it at the provider immediately. Deleting
  the line does not remove it from git history.

## Deployments

| Trigger | Path |
|---|---|
| Push to `main` | Backend image published to GHCR (`docker-publish.yml`) |
| Push to `staging` | Deploy to the staging ECS cluster (`deploy-staging.yml`) |
| Published GitHub release | Deploy to the production ECS cluster (`deploy-production.yml`) |
| Push to `main` | Deploy to the EC2 host (`deploy-ec2.yml`) |
| Tag `v*.*.*` | Cross-platform CLI release (`cli-release.yml`) |

Production deploys require a GitHub release, and the manual production
workflow requires the word `DEPLOY` typed into its confirmation field. The
default for that field is deliberately empty so the gate can actually refuse.

## Adding a dependency

```bash
go get <module>@<version>
go mod tidy
```

`pr-quality-gate.yml` fails if `go mod tidy` changes `go.mod` or `go.sum`,
so run it locally before pushing. Dependabot opens weekly updates for Go
modules, GitHub Actions and Docker base images.

## Reporting security issues

Do not open a public issue. See [`SECURITY.md`](SECURITY.md).
