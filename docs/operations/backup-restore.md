# Backup, Restore & Disaster Recovery (REQ-8.5)

## Targets

| Tier | RPO | RTO | Scope |
|---|---|---|---|
| Team/SaaS | ≤ 1h (PG PITR) | ≤ 4h | PG + Redis snapshot + queue depth note |
| Scale | ≤ 15min | ≤ 1h | Above + standby PG + mirrored queues |
| Enterprise / Global Monorepo | ≤ 5min | ≤ 30min | Above + tested AZ failover (runbook below) |

Targets are contractual only at the tier that pays for the standby infrastructure; below that they are best-effort with the same procedures.

## What gets backed up (and why each matters)

| Store | Contents | Method | Restore test |
|---|---|---|---|
| PostgreSQL | Licenses, seats, audit logs, review history, warehouse + rollups | PITR (WAL archiving) + daily base backup | Quarterly restore to staging + seat-count reconciliation query (must match pre-backup; double-counting = failed test) |
| Redis | Locks, idempotency keys, quotas, rate-limit buckets | Snapshot (RDB) + AOF per deployment | Restore + duplicate-webhook replay test (must dedup, not double-execute) |
| RabbitMQ | In-flight review jobs, DLQ contents | Durable quorum queues (no separate backup); DLQ drained to PG before maintenance | Post-restore queue-depth + DLQ-content comparison |
| Object storage | Review artifacts, SARIF | Versioned bucket replication | Spot-restore + hash check |
| Secrets | All rotation-runbook material | Secret-manager versioning (never in backups as plaintext) | Access drill, not content drill |

## Seat/quota integrity across failover (normative)

Seat state restores from PG backup only — never reconstructed from worker memory or beacon caches. After any restore: run the seat reconciliation (sum of active seats vs license `MaxSeats` per workspace); over-quota workspaces are frozen for new provisions (409) until reconciled, not silently allowed.

## Restore runbook (quarterly drill, recorded)

1. Declare drill, freeze non-prod writes to the restore target.
2. Restore PG to target time; verify migration level matches binary version (see `upgrade-migration.md` — never restore a newer DB under an older binary).
3. Restore Redis snapshot; replay a captured duplicate webhook; assert single execution.
4. Reconcile seats; assert counts.
5. Record: date, operators, RPO achieved, RTO achieved, anomalies. A drill that isn't recorded didn't happen.
