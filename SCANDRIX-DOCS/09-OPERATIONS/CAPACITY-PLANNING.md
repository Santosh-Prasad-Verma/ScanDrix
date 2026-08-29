# Enterprise Capacity Planning & Resource Sizing Guide

**Classification:** AUTHORITATIVE OPERATIONAL SPECIFICATION  
**Status:** APPROVED  
**Target Version:** v1.0 Enterprise  
**Domain:** Site Reliability Engineering & Infrastructure Architecture  

---

## 1. Executive Sizing Model & Workload Assumptions

Scandrix is engineered to support enterprise workloads from 1,000 to 100,000 pull requests per day across thousands of repositories. This guide provides quantitative baseline formulas for sizing Kubernetes compute, dedicated sandbox hardware, PostgreSQL database storage, and messaging throughput.

### Standard Workload Reference Tiers

| Parameter | Mid-Market Baseline | Enterprise Tier 1 | Mega-Monorepo / FinTech |
|---|---|---|---|
| **PRs / Day** | $1,000$ | $10,000$ | $50,000+$ |
| **Peak Ingestion Rate** | $15\text{ webhooks/sec}$ | $150\text{ webhooks/sec}$ | $1,000+\text{ webhooks/sec}$ |
| **Average Diff Size** | $250\text{ LOC}$ (4 files) | $500\text{ LOC}$ (12 files) | $2,500+\text{ LOC}$ (80+ files) |
| **Target P95 Scan Latency** | $< 20\text{ seconds}$ | $< 30\text{ seconds}$ | $< 60\text{ seconds}$ |

---

## 2. Kubernetes Compute & Worker Pool Sizing

### 2.1 Stateless Core API Gateways (`scandrix-api`)
- **Resource Request per Pod**: 1.0 vCPU, 1 GB RAM.
- **Resource Limit per Pod**: 4.0 vCPU, 4 GB RAM.
- **Throughput Capacity**: Each Go 1.24+ Chi pod comfortably processes $\sim 800\text{ req/sec}$ for simple REST/outbox commits.
- **Replica Recommendation**:
  - Baseline: 6 pods (2 per AZ across 3 AZs for N+2 redundancy).
  - Scaled: Up to 30 pods under peak webhook bursts.

### 2.2 Scan Analysis Workers (`scandrix-worker`)
- **Resource Request per Pod**: 2.0 vCPU, 4 GB RAM.
- **Concurrency**: 8 parallel AST analysis goroutines per worker pod.
- **Autoscaling Rule (KEDA)**:
  $$\text{TargetReplicas} = \min\left(100, \max\left(5, \left\lceil \frac{\text{QueueDepth}}{5} \right\rceil\right)\right)$$
  Spins up 1 worker pod for every 5 queued scan DAG tasks in `q.jobs.scan.dag`.

---

## 3. Dedicated Sandbox Execution Pool (gVisor & Firecracker)

To isolate untrusted test suites and linters, sandboxes run on dedicated bare-metal Kubernetes worker nodes (`.spec.nodeSelector: instance-type: metal`):

### 3.1 Tier 1: gVisor Containers (`runsc`)
- **Density**: 40 concurrent gVisor containers per 64-core node (`c6i.metal` or equivalent).
- **Per-Container Quotas**: 1 vCPU, 512 MB RAM, 256 MB writable `tmpfs` at `/tmp`.
- **Boot Overhead**: $<350\text{ms}$ startup, negligible host memory footprint.

### 3.2 Tier 2: Firecracker MicroVMs (Hardware KVM)
- **Node Requirement**: Direct `/dev/kvm` hardware access (bare metal or nested virtualization enabled).
- **Density**: Up to 60 concurrent MicroVMs per 128 GB RAM host node.
- **Per-MicroVM Quotas**: 2 vCPUs, 1024 MB RAM, 2 GB ephemeral copy-on-write ext4 rootfs disk snapshot.
- **Pre-warmed Pool**: Maintain a standby pool of $\ge 15$ pre-warmed idle MicroVMs to guarantee $<80\text{ms}$ test boot turnaround.

---

## 4. PostgreSQL Database & Storage Projections

### 4.1 Storage Growth Formula
For an enterprise processing $N$ pull requests per day:

$$\text{DailyStorage} = N \times \left( \text{Size}_{\text{scan\_record}} + 8 \times \text{Size}_{\text{finding}} + 18 \times \text{Size}_{\text{evidence\_packet}} \right)$$

- Average scan metadata + findings + SHA-256 evidence packets $\approx 85\text{ KB}$ per PR.
- **At 10,000 PRs/day**: $\approx 850\text{ MB/day} \implies \approx 25.5\text{ GB/month}$.
- **pgvector Embedding Index**: $\approx 6.5\text{ KB}$ per security memory record (1536 floats $\times 4\text{ bytes} + \text{HNSW graph overhead}$).
- **Partitioning Policy**: Monthly partitions with automatic archival to Amazon S3 / cold storage after 18 months, maintaining active PostgreSQL database size $< 500\text{ GB}$.

### 4.2 IOPS Requirements
- Baseline: AWS RDS `gp3` with 12,000 IOPS and 500 MB/s throughput.
- Connection Pooling: PgBouncer configured with `pool_mode = transaction` to sustain 5,000 active client connections multiplexed into 150 dedicated backend PostgreSQL connections.

---

## 5. RabbitMQ Messaging Capacity

- **Quorum Queue Disk Throughput**: 10,000 messages/sec sustained on NVMe SSD storage with synchronous Raft replication across 3 nodes.
- **Message Payload Sizing**: Raw webhook payloads are stored in the database outbox; RabbitMQ messages carry only compact pointer references ($\approx 450\text{ bytes}$ per AMQP message).
- **Memory Ceiling**: RabbitMQ cluster configured with `vm_memory_high_watermark.relative = 0.6` (60% RAM threshold) with automatic paging to disk.
