# ADR 0003: Supabase PostgreSQL & pgvector as Canonical Data Store

**Classification:** ARCHITECTURE DECISION RECORD  
**Status:** APPROVED  
**Date:** 2026-08-28  
**Deciders:** Scandrix Core Architecture Team  

---

## 1. Context & Problem Statement

Scandrix requires a robust relational database to enforce multi-tenant Row-Level Security (RLS), store immutable Merkle evidence packets, and query high-dimensional vector embeddings for semantic security memory (historical false positives and architecture rules).

---

## 2. Considered Options

1. **Supabase PostgreSQL 16+ with `pgvector`**: ACID-compliant transactional guarantees, native PostgreSQL Row-Level Security, HNSW vector indexing, mature tooling, self-hostable.
2. **MongoDB + Dedicated Vector DB (Pinecone / Qdrant)**: Dual-database architecture adds operational complexity, cross-database transaction failure risks, and higher hosting overhead.
3. **CockroachDB / Spanner**: High cost and complexity unnecessary for standard multi-tenant enterprise scale.

---

## 3. Decision Outcome

We select **Supabase PostgreSQL with the `pgvector` extension** as the single source of truth for all transactional, relational, and vector data.

### Key Justifications:
- **Unified Transactional Integrity**: Vector embeddings and evidence packets reside in the same ACID database, allowing atomic rollbacks if scan processing fails.
- **Kernel-Level Multi-Tenancy**: PostgreSQL Row-Level Security (RLS) guarantees that tenant isolation is enforced inside the database engine itself.
- **HNSW Vector Search**: Delivers sub-10ms nearest-neighbor semantic search over security findings directly in SQL queries.

---

## 4. Consequences

- **Positive**: Single database to back up, monitor, and scale; zero distributed transaction synchronization bugs; native RLS tenant shielding.
- **Trade-off**: Requires careful connection pooling (PgBouncer) to prevent connection saturation under bursty webhook loads.
