# Scandrix — Entity Relationship Diagram (ERD) Specification

**Classification:** AUTHORITATIVE SPECIFICATION  
**Status:** APPROVED  
**Version:** 2.0.0  
**Database Engine:** PostgreSQL 16+ (Supabase) with `pgvector` extension

---

## 1. Executive Summary & Relational Schema Topology

The Scandrix relational schema is partitioned across four core functional domains:
1. **Multi-Tenant Hierarchy & Repositories**: Organization boundaries, workspaces, and Git provider metadata.
2. **Assurance Execution & Evidence**: Scan runs, DAG stage execution records, deterministic findings, and immutable Merkle-tree rooted evidence packets.
3. **Graph & Attack Surface Intelligence**: Code-to-Cloud attack nodes, weighted friction edges, and shortest-path Dijkstra traversals.
4. **Governance, AI & Audit**: Semantic security memory (pgvector), policy evaluation decisions, remediation proof-of-fix attestations, and cryptographic action ledgers.

```mermaid

erDiagram
    tenants ||--o{ workspaces : contains
    workspaces ||--o{ repositories : manages
    repositories ||--o{ scan_runs : executes
    scan_runs ||--o{ scan_stage_results : tracks
    scan_runs ||--o{ findings : produces
    
    findings ||--|| evidence_packets : backed_by
    findings ||--o| remediation_records : resolves
    
    repositories ||--o{ attack_nodes : contains
    attack_nodes ||--o{ attack_edges : connects_to
    
    tenants ||--o{ policies : enforces
    policies ||--o{ policy_evaluations : evaluates
    scan_runs ||--o{ policy_evaluations : assessed_by
    
    tenants ||--o{ security_memory : retains
    tenants ||--o{ agent_action_ledger : audits
    scan_runs ||--o{ assurance_manifests : certifies

    tenants {
        uuid id PK
        string slug UK
        string name
        string tier
        timestamptz created_at
    }

    workspaces {
        uuid id PK
        uuid tenant_id FK
        string name
        string slug
    }

    repositories {
        uuid id PK
        uuid workspace_id FK
        string provider
        string remote_url
        string default_branch
    }

    scan_runs {
        uuid id PK
        uuid repository_id FK
        string commit_sha
        string base_sha
        string status
        numeric composite_risk_score
        timestamptz started_at
        timestamptz completed_at
    }

    scan_stage_results {
        uuid id PK
        uuid scan_run_id FK
        string stage_name
        string status
        int duration_ms
    }

    findings {
        uuid id PK
        uuid scan_run_id FK
        string rule_id
        string severity
        string file_path
        int start_line
        int end_line
        string status
    }

    evidence_packets {
        uuid id PK
        string packet_id UK
        string commit_sha
        string ast_node_path
        char64 evidence_hash
        jsonb payload
        timestamptz recorded_at
    }

    remediation_records {
        uuid id PK
        uuid finding_id FK
        string status
        text unified_diff
        boolean compilation_passed
        boolean tests_passed
        string proof_of_fix_sig
    }

    attack_nodes {
        uuid id PK
        uuid repository_id FK
        string node_type
        string label
        string file_path
        int line_number
    }

    attack_edges {
        uuid id PK
        uuid source_node_id FK
        uuid target_node_id FK
        string relation_type
        numeric weight_friction
    }

    security_memory {
        uuid id PK
        uuid tenant_id FK
        string entity_type
        vector embedding
        text content
    }

    assurance_manifests {
        uuid id PK
        uuid scan_run_id FK
        string assurance_level
        string image_digest
        text in_toto_statement
        string signature
    }

```

---

## 2. Cardinality & Relational Invariants

1. **Evidence Immutability**: Every `findings` record has a strict $1:1$ relationship with an `evidence_packets` record. Updating or deleting an `evidence_packets` row is prohibited by PostgreSQL database rules.
2. **Tenant Scoping**: All operational tables possess a foreign key reference or inherited composite key bound to `tenant_id`, guaranteeing PostgreSQL Row-Level Security (RLS) enforcement.
3. **Graph Referential Integrity**: Deleting a repository cascades to its associated `attack_nodes` and `attack_edges`, preventing orphaned graph states in the Attack Path Engine.
