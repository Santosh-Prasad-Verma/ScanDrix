# Operational Runbooks

One page per incident. Each runbook: symptoms → diagnosis → action → verify → post-mortem trigger. Severity definitions live in `slo-error-budgets.md`.

## 1. Queue drain / DLQ replay

- **Symptoms:** review latency SLO breach, DLQ depth alert, worker lag.
- **Diagnose:** queue depth per queue, consumer count/prefetch, DLQ head message class (poison payload vs downstream outage — inspect, don't blindly replay).
- **Action:** scale workers within tier sizing (TRD §4) → fix root cause → replay DLQ selectively (by class, oldest first) → purge only with written approval + record of purged IDs.
- **Verify:** depth returns under threshold for 30min; spot-check replayed reviews posted exactly once (inbox dedup is the backstop, not the plan).
- **Post-mortem:** any purge, any replay >1000 messages, any recurrence within 7 days.

## 2. Webhook flood / duplicate storm

- **Symptoms:** 429s to providers, Redis lock contention, duplicate reviews posting.
- **Diagnose:** distinguish provider retries (same delivery IDs → dedup working) from distinct events (real storm).
- **Action:** confirm HMAC still enforced (never disable verification to "reduce load"); scale webhooks tier; extend dedup TTL temporarily (recorded change, reverted after).
- **Verify:** duplicate rate back under 1%; no double-posted comments in sampled PRs.

## 3. License expiry / invalid license at boot

- **Symptoms:** service refuses boot (`invalid license configuration`), or entitlements flip to Community after expiry+grace.
- **Diagnose:** `verify` the token (Phase 1 tooling) against the configured public key; check `ExpiresAt` vs now vs 7-day grace; check `KeyID` against registered ring.
- **Action:** install renewed license (KEY or FILE, never both); rolling restart; confirm `/capabilities` shows expected tier before opening traffic.
- **Verify:** feature gates pass for entitled flags; seats intact (reconcile per `backup-restore.md`).
- **Post-mortem:** any production expiry without 30-day advance notice (alerting gap).

## 4. Secret compromise (suspected or confirmed)

- **Action order:** contain (revoke at provider / IdP first) → rotate per `rotation-runbooks.md` → audit (which systems accepted the old secret in the window; audit-log query) → notify per breach SLA (`security/dpa-support.md`).
- **Never:** delete logs, rotate before containment, or paste the secret into chat/tickets.

## 5. AZ / node failover (Scale+ tiers)

- **Preconditions:** standby PG, mirrored quorum queues, tested restore (quarterly drill record exists).
- **Action:** promote standby → point services → reconcile seats → drain-and-verify queues → reopen traffic in stages (webhooks last, after ingestion verified).
- **Verify:** SLO dashboards green for 1h; seat reconciliation matches; DLQ empty of failover-period messages.

## 6. Sandbox outage

- Per PRD REQ-2.3 degraded mode: findings post `unverified (sandbox unavailable)` — this is correct behavior, not an incident to "fix" by disabling verification labels. Incident = restore sandbox capacity; never bulk-relabel unverified findings as verified.
