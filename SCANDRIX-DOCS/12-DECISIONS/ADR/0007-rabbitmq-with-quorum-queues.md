# ADR 0007: RabbitMQ with Quorum Queues as Asynchronous Message Broker

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-29  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

Scandrix requires a rock-solid, resilient asynchronous messaging substrate to decouple sub-15ms webhook ingestion from long-running 18-stage assurance scans, gVisor/Firecracker sandbox jobs, and autonomous patch remediation tasks. The messaging broker must provide guaranteed at-least-once delivery, strict message deduplication support, high-availability clustering without data loss under network partitions, and native KEDA autoscaling support in Kubernetes.

---

## 2. Considered Options

1. **RabbitMQ (with Quorum Queues & Delayed Exchange Plugin)**:
   - Raft-based consensus protocol providing strong data safety and partition tolerance.
   - Rich AMQP 0-9-1 routing model (topic exchanges, dead-letter exchanges, per-message TTL, delayed retries).
   - Battle-tested KEDA native scaler (`type: rabbitmq`, `mode: QueueLength`).
   - Standard client library in Go (`github.com/rabbitmq/amqp091-go`).
2. **NATS JetStream**:
   - Ultra-lightweight and native to Go, but smaller enterprise operational tooling footprint compared to RabbitMQ in legacy banking/enterprise client environments, and less granular dead-letter / delayed exchange semantics out of the box.
3. **Apache Kafka**:
   - Exceptional log streaming throughput for event streaming, but high operational complexity (JVM/ZooKeeper or KRaft overhead), partition-key head-of-line blocking for heterogeneous workloads (fast AST vs 3-minute sandbox test), and poor fit for individual task queue consumption with fine-grained per-job acknowledgments.
4. **Redis Streams**:
   - In-memory speed, but lacks native quorum-based disk durability guarantees and advanced routing/dead-lettering required for mission-critical enterprise compliance audits.

---

## 3. Decision Outcome

We select **RabbitMQ 3.13+ with Quorum Queues and the Delayed Message Exchange Plugin**.

### Key Architectural Justifications:
- **Raft Consensus Safety**: Quorum queues utilize the Raft consensus algorithm, guaranteeing zero data loss even if individual cluster nodes abruptly fail during high-concurrency PR events.
- **Dead-Lettering & Exponential Backoff**: Native dead-letter routing (`x-dead-letter-exchange`) paired with delayed message exchanges enables resilient 5-stage exponential retry policies without custom scheduler infrastructure.
- **KEDA Native Autoscaling**: Seamless integration with Kubernetes Event-driven Autoscaling scales worker pods from 5 to 100 replicas based directly on queue depth.
- **Decoupled Architecture**: Combined with the Transactional Outbox pattern in PostgreSQL, RabbitMQ ensures total delivery idempotency and fire-and-forget sub-15ms ingestion.

---

## 4. Consequences

- **Positive**: Resilient, cluster-wide message safety; automated KEDA worker scaling; mature Go driver support; native DLQ and delayed retries.
- **Trade-off**: Requires dedicated RabbitMQ cluster management (or managed AWS MQ / CloudAMQP) with mTLS encryption and periodic Erlang runtime monitoring.
