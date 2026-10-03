# Egress Allowlist & Audit Procedure (PRD §6 gate)

## 1. Allowlist (default-deny; everything not listed is a finding)

| Destination class | SaaS | Self-hosted | Air-gapped |
|---|---|---|---|
| Customer SCM APIs (GitHub/GitLab/Bitbucket/Azure/Forgejo) | ✅ | ✅ customer endpoints | ✅ customer endpoints |
| Customer IdP (SAML/OIDC/SCIM callbacks) | ✅ | ✅ customer IdP | ✅ customer IdP |
| Configured LLM providers (per workspace routing) | ✅ | ✅ or local inference | ❌ local inference only |
| Razorpay (billing) | ✅ | If billing enabled | ❌ |
| Sentry | If `SENTRY_DSN` set | If set | ❌ (no DSN = inert, verified) |
| Beacon (`SCANDRIX_TELEMETRY_ENDPOINT`) | ✅ | Honor `SCANDRIX_TELEMETRY_DISABLED` | ❌ disabled + verified |
| PostHog | Cloud path only | ❌ | ❌ |
| Container registries / OS mirrors (build-time only) | ✅ | ✅ | ❌ (offline bundle flow, REQ-8.7) |

## 2. Quarterly audit (recorded, signed)

1. Enable `AIR_GAPPED=true` enforcement (**[IMPLEMENTED]** in `internal/platform/security/airgap.go`) plus deployment firewalling in staging-mirror.
2. Run the egress test (`TestAirGapGate_BlocksExternalWhenAirGapped` and `TestAirGappedEgress`) plus 24h packet capture on all nodes.
3. Classify every external flow against §1; unlisted flows are SEV-2 findings with owners and dates.
4. Sign and file the report; PRD §6 scorecard reads the latest report, never an older one.

## 3. Exemptions

Allowlisted VPC endpoints (private LLM gateways, internal mirrors) are listed per deployment with owner + review date. An exemption without an expiry date is a finding.
