# SLOs, Error Budgets & Paging

Source numbers: PRD §5. This doc turns them into alerts and on-call behavior. Budgets are monthly; a burned budget freezes non-urgent deploys for the affected surface until recovery — that is the entire point of the budget.

## SLO table

| SLO | Target | Measure | Alert (page) | Alert (ticket) |
|---|---|---|---|---|
| Review latency P90 | < 45s (NFR envelope) | `review.duration_seconds` histogram, 7-day window | P90 > 60s for 30min | P90 > 45s for 6h |
| Review latency P50 | < 20s | Same | — | P50 > 20s for 24h |
| Dashboard queries P99 | < 120ms | Rollup-table query timings, staging-seeded + prod | P99 > 250ms for 15min | P99 > 120ms for 6h |
| Worker RSS P95 | < 350MB | Container metrics | > 500MB for 15min (OOM risk) | > 350MB for 24h |
| Webhook ingestion | 99.9% accepted < 2s (excl. provider outages) | Gateway timings + 5xx rate | 5xx > 1% for 10min | 5xx > 0.1% for 1h |
| Queue freshness | Consumer lag < 5min | Queue depth / consume rate | Lag > 15min | Lag > 5min for 1h |
| Uptime (single-node) | 99.9% | Successful health checks | — | Monthly review |
| Uptime (clustered) | 99.95% | Same, multi-AZ | Any AZ-failover event pages | Monthly review |

## Severity + paging

- **SEV-1 (page immediately):** data-loss risk, tenant-isolation failure, license fail-closed fleet-wide, sandbox escape, secret compromise. Work stops; incident commander; post-mortem mandatory.
- **SEV-2 (page in hours):** SLO breach with user impact, DLQ growth, single-provider webhook outage. Post-mortem if user-visible.
- **SEV-3 (ticket):** threshold warnings, single-customer issues, docs gaps found in incidents.

## Budget policy

- Error budget = 100% − SLO over 30 days, tracked per SLO above.
- Budget exhausted → feature deploys on that surface stop; only fixes, rollbacks, and capacity ship until the budget recovers for 7 consecutive days.
- Budget exemptions require written sign-off naming the expiry date; permanent exemptions are forbidden (fix the SLO instead, via ADR).
