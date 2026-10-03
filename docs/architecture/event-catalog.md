# Event & Queue Catalog

Every asynchronous boundary in the system. RabbitMQ topology is declared in `internal/queue/rabbitmq.go`; reliability comes from the transactional outbox (`queue/relay/outbox.go`) + inbox dedup (`queue/relay/inbox.go`), not from the broker alone.

## 1. Topology

| Kind | Name | Type | Consumers | DLX |
|---|---|---|---|---|
| Queue | `scandrix.reviews.v1` | quorum, durable | `scandrix-worker` (prefetch 16, worker pool) | `scandrix.dlx` → `scandrix.reviews.v1.dlq` |
| Queue | `scandrix.billing.v1` | quorum, durable | `scandrix-worker` (billing consumer) | `scandrix.dlx` → `scandrix.billing.v1.dlq` |
| Queue | `scandrix.sandbox.invalidate.v1` | quorum, durable | `scandrix-worker` (fanned out for multi-node clusters) | `scandrix.dlx` → `….dlq` |
| Exchange | `scandrix.dlx` | dead-letter | — | — |
| Exchange | `scandrix.delayed` | delayed-message (backoff) | publisher-side | — |

Messages are published persistent (`DeliveryMode: amqp.Persistent`). Publisher confirms are **SPECCED** (not yet asserted in `internal/queue` — see `enterprise/TRD.md` §2).

## 2. Payloads

**Review task** (`queue/consumer.ReviewTaskPayload`):

| Field | Purpose |
|---|---|
| `task_id`, `event_id` | idempotency + trace correlation |
| `workspace_id` | tenant scope for the job |
| `provider` | SCM adapter to use |
| `repo_namespace`, `pull_request_number`, `head_sha`, `base_sha` | what to review |
| `sender` | actor for attribution/audit |
| `attempt_count`, `enqueued_at` | retry bookkeeping |

**Sandbox invalidate** (`sandbox/contracts.SandboxInvalidatePayload`, routing key `sandbox.invalidate`): `pr_key`, `reason`. Consumed in-process by the relay; published to RabbitMQ only for multi-node clusters.

**Billing event** (`billing/events.go`): 7 lifecycle events (subscription created/updated/cancelled, payment succeeded/failed, invoice issued, plan changed) — consumed on `scandrix.billing.v1`.

## 3. Reliability contracts

**Outbox** (`outbox_events`): written in the same transaction as the state change, then relayed. States `PENDING → PUBLISHED | FAILED | DEAD_LETTER`. Relay defaults (`DefaultRelayConfig`): batch 50, poll 100ms, claim 30s, **max 5 retries**, exponential backoff 100ms → 10s. Priority per plan tier so paid workspaces aren't starved.

**Inbox** (`inbox_records`): per-consumer claim keyed by `message_id`; statuses `PROCESSING / COMPLETED / FAILED / RETRY`. A duplicate delivery is acknowledged, never re-executed.

**Dead-letter policy:** after 5 attempts a message is rejected to the DLQ and stays there until an operator acts — see `operations/runbooks.md` §1. Nothing auto-purges a DLQ.

## 4. Producer/consumer matrix

| Producer | Event | Consumer | Failure handling |
|---|---|---|---|
| `webhooks` | PR event → `outbox_events` | relay → `reviews.v1` → worker | 5 retries → DLQ; webhook already acked 200 |
| `webhooks` (billing) | payment event → outbox | `billing.v1` → billing service | idempotency store dedups replays |
| worker (lease changes) | `sandbox.invalidate` | lease manager (in-process) | reaper cron is the backstop |
| worker (crons) | DORA rollups, spend alerts, seat prune | direct DB writes | cron logs + alerts; no queue hop |
| SCM callbacks | PR status updates | `webhooks` | dedup key `repo:pr:sha` |

## 5. Debugging in order

1. Was the event written? `outbox_events` for the workspace (state, retry_count, last_error).
2. Did the relay publish it? Relay logs + `OutboxLagStats` (pending, claimed, oldest pending).
3. Did the broker accept it? Queue depth + consumer count.
4. Did the consumer claim it? `inbox_records` status.
5. Did the work fail? `last_error` on the inbox record, then the DLQ for exhausted retries.

Metrics names for each step are in `operations/slo-error-budgets.md` (queue freshness, DLQ depth).
