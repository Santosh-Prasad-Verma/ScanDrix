# Deployment Topology

Four modes from one build. What runs where, network boundaries, ports, and scaling units. Compose files (`docker-compose*.yml`) and `infra/terraform/` are the executable truth; this doc is the map and the rationale.

## 1. Modes

| Mode | Who operates | Components | License path | Notes |
|---|---|---|---|---|
| **SaaS** | ScanDrix | Managed PG/Redis/RabbitMQ + containers | plan rows in `plan_configurations` + online billing | PostHog decision path active (`featuregate` cloud branch) |
| **Dedicated** | ScanDrix, single tenant | Same as SaaS, isolated project | plan row | Region pinning per order |
| **Self-hosted (VPC/on-prem)** | Customer | Customer PG/Redis/RabbitMQ/S3 + our containers | signed license file (`SCANDRIX_LICENSE_FILE`) | Cloud-only features (funded provider, trials) off; OIDC domain-verification relaxed for private hostnames |
| **Air-gapped** | Customer, no egress | As self-hosted + local inference (vLLM/Ollama) | signed license, optional hardware binding | No external LLM/telemetry/billing egress; enforcement gate is **IMPLEMENTED** (`AirGapGate` in `internal/platform/security/airgap.go`) + network perimeter controls |

## 2. Component placement

| Component | SaaS | Self-hosted / air-gapped | Scaling unit |
|---|---|---|---|
| `scandrix-api` | 2+ replicas behind LB | 1–3 (HA optional) | vertical; stateless |
| `scandrix-webhooks` | 2+ | 1–2 | vertical; stateless |
| `scandrix-worker` | 2–16 (tier-sized, TRD §4) | 2–8 | **horizontal** — the only tiered unit |
| `scandrix-server` (monolith) | dev/small | dev/small | replaces api+webhooks+crons |
| `scandrix-mcp-manager` | optional | optional | stateless |
| PostgreSQL 16+ (pgvector) | managed, multi-AZ | customer | vertical + read replicas |
| RabbitMQ 3.12+ (delayed plugin) | managed | customer | quorum queues (3-node for HA) |
| Redis 7+ | managed | customer | single primary is enough |
| S3/Appwrite | ScanDrix storage | customer bucket | object storage |

Worker sizing (TRD §4): 50–250 devs → 2 workers (4 vCPU/8 GB); 250–1000 → 4 (8/16); 1000+ → 8–16 (16/32). API/webhooks stay flat until the worker tier is maxed.

## 3. Network boundaries & ports

| Port | Service | Exposure |
|---|---|---|
| `8080` | api / server | public (behind TLS terminator) |
| `8081` | webhooks | public, provider IPs allowed |
| `8082` | worker health (also `try` default — co-location needs `WORKER_HEALTH_PORT`) | internal |
| `3101` | mcp-manager (flag-configurable) | internal |
| `5432` | PostgreSQL | internal only, TLS |
| `5672` / `15672` | RabbitMQ AMQP / mgmt | internal only; mgmt never public |
| `6379` | Redis | internal only |
| `8388` | E2B proxy (if used) | internal |

Rules: TLS everywhere in prod (`sslmode=require`); mgmt UIs bound to internal network or VPN; no database port reachable from the internet; air-gapped networks carry no route to public DNS.

## 4. Infrastructure as code

- `infra/terraform/` — VPC, ALB, ECS services, ElastiCache, CloudFront, Route53, CloudWatch, IAM, Secrets Manager; `use_localstack` flag for a local rehearsal.
- `docker-compose.yml` — production-shaped single-host (PG + RabbitMQ + Redis + migrate + api + webhooks + worker).
- `docker-compose.dev.yml` — dev defaults; `docker-compose.cluster.yml` — 2 api + 4 workers + LocalStack + mcp-manager to rehearse the AWS layout locally.

Deploy order: `migrate` (job, must succeed) → api/webhooks → workers. Never roll a worker build ahead of the migration it expects (`operations/upgrade-migration.md`).

## 5. Failure domains

| Failure | Blast radius | Mitigation |
|---|---|---|
| api replica loss | requests only | LB + stateless restarts |
| worker loss | in-flight reviews | quorum queue redelivery + inbox dedup (no double-execution) |
| PG loss | everything | PITR + restore drill (`operations/backup-restore.md`); seat state restores from PG only |
| Redis loss | locks/quota/cache | rebuilds; duplicate-webhook protection degrades to PG inbox (slower) |
| RabbitMQ loss | async path | durable quorum queues; outbox retains unpublished work until the relay reconnects |
| Bad license | boot refusal (by design) | `operations/runbooks.md` §3 |
