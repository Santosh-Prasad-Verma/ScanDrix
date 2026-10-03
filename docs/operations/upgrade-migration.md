# Upgrade & Migration Guide

## Migration numbering (normative)

- `migrations/NNN_*.sql` sequential; next free number is **039** (001–038 taken).
- One migration = one transactional unit, reversible or explicitly marked irreversible with the forward-only reason.
- `cmd/migrate` applies in order with `schema_migrations` tracking; never hand-apply out of order.
- Rollback rule: every migration ships its down-path or a documented restore-from-backup path. Code and schema deploy together — **never restore a newer DB under an older binary** (checked in `backup-restore.md` drill step 2).

## Compatibility matrix (maintained per release)

| Component | Compatible with | Notes |
|---|---|---|
| Dashboard → API | Same minor version; additive API fields only | Breaking API changes require a dated note in `reference/api-reference.md` |
| Worker → API/DB | Same release tag | Queue envelope version checked at consume; mismatched envelopes go to DLQ, not crash-loop |
| `cmd/migrate` | Exactly one release ahead max | Skipped-version upgrades go through each intermediate migration in order |

## Release checklist

1. Migrations reviewed (reversible or marked), numbered, applied to staging + seat-reconciliation green.
2. `go-licenses` gate green (IMPLEMENTATION §3.4); `gosec` green on touched trees.
3. Eval gate for any model/prompt/threshold change (methodology doc).
4. Egress test green for air-gap-affecting changes.
5. Dashboard↔API compat row updated; customer-facing changes flagged for changelog.
6. Rollback path rehearsed or explicitly accepted as restore-from-backup (recorded).
