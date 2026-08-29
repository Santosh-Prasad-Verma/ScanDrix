# Scandrix — Production PostgreSQL 16+ Data Model

**Status:** Authoritative production DDL  
**Database:** Supabase PostgreSQL 16+ with `pgvector`  
**Isolation model:** one application transaction, one verified tenant context, enforced by Row-Level Security (RLS)

This schema is the persistent boundary for Scandrix' deterministic-assurance model. A model response may be stored as analysis, but it cannot independently create a `CONFIRMED` finding: confirmation requires a cryptographically verifiable `evidence_packets` row. Likewise, a review never maps an unrun stage to success; every stage records one of the six explicit analysis outcomes.

## 1. Schema invariants

| Invariant | Database enforcement |
| --- | --- |
| Tenant isolation | Every tenant-owned table has `tenant_id`, RLS is enabled and forced, and both read and write predicates use `app.current_tenant_id()`. |
| Transaction-local context | The API sets `app.current_tenant_id` with `SET LOCAL` / `set_config(..., true)` inside every transaction. An unset context produces no visible rows and no writable rows. |
| Partition correctness | `scans` is range partitioned by `created_at`; its primary and foreign keys include the partition key, as PostgreSQL requires. |
| Status precision | `analysis_outcome` has exactly `PASS`, `PASS_WITH_WARNINGS`, `FAIL`, `PARTIAL_ANALYSIS`, `NOT_TESTED`, and `ERROR`. |
| Deterministic confirmation | A `CONFIRMED` finding must reference an evidence packet in the same tenant and scan. Evidence packets retain SHA-256/Merkle identities and are immutable. |
| Agent firewall | Tier-4 ledger entries require signed human authorization metadata. The append-only ledger rejects `UPDATE` and `DELETE`. |
| Security-memory retrieval | `security_memory.embedding` is exactly `vector(1536)` and has an HNSW cosine-distance index using `m = 16` and `ef_construction = 64`. |

The migration role owns schema objects. The runtime role (`scandrix_app`) is a non-superuser, does not own tenant tables, does not have `BYPASSRLS`, and is the only role used by API and worker request paths. Supabase service-role credentials and direct browser database access are not used for application queries.

## 2. Authoritative migration

Run this migration with a privileged migration role before granting the runtime role access. It is intentionally self-contained: `pgvector`, all enum types, indexes, RLS policies, append-only guards, and the rolling partition function are defined here.

```sql
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS app;

-- PostgreSQL does not provide CREATE TYPE IF NOT EXISTS for all enum releases.
DO $$
BEGIN
    CREATE TYPE scan_trigger AS ENUM (
        'PULL_REQUEST', 'PUSH', 'CLI_STAGED', 'SCHEDULED', 'MANUAL', 'WEBHOOK_REPLAY'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE scan_state AS ENUM (
        'QUEUED', 'RUNNING', 'COMPLETED', 'CANCELLED'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE analysis_outcome AS ENUM (
        'PASS', 'PASS_WITH_WARNINGS', 'FAIL', 'PARTIAL_ANALYSIS', 'NOT_TESTED', 'ERROR'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

-- Note: The 11 analysis_stage categories below group the 18 granular stages
-- of the Scandrix Assurance DAG into persistent domain aggregations for storage
-- and reporting (e.g., stages 2-4 map to DEPENDENCY_RESOLUTION, 5-7 map to SAST).
DO $$
BEGIN
    CREATE TYPE analysis_stage AS ENUM (
        'INGESTION', 'DEPENDENCY_RESOLUTION', 'SAST', 'SCA', 'SECRETS', 'IAC',
        'REACHABILITY', 'DAST', 'POLICY_EVALUATION', 'REMEDIATION_VERIFICATION',
        'ASSURANCE_MANIFEST'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE finding_status AS ENUM (
        'OBSERVED', 'CONFIRMED', 'SUPPRESSED', 'ACCEPTED_RISK', 'REMEDIATED', 'FALSE_POSITIVE'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE evidence_kind AS ENUM (
        'AST_PATH', 'TAINT_DATAFLOW', 'DEPENDENCY_LOCK', 'LIVE_REACHABILITY',
        'REPRODUCTION_RECORD', 'SIGNED_EXTERNAL_ATTESTATION'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE remediation_state AS ENUM (
        'REQUESTED', 'SYNTHESIZING', 'VERIFYING', 'VERIFIED', 'REJECTED', 'ERROR'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

DO $$
BEGIN
    CREATE TYPE agent_action_tier AS ENUM (
        'TIER_1_READ', 'TIER_2_ANALYZE', 'TIER_3_PROBE', 'TIER_4_MUTATE'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END;
$$;

-- Returns NULL when the request path has not set tenant context. RLS treats the
-- resulting comparison as false, which is deny-by-default without leaking rows.
CREATE OR REPLACE FUNCTION app.current_tenant_id()
RETURNS uuid
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
    SELECT NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION app.require_tenant_context()
RETURNS uuid
LANGUAGE plpgsql
STABLE
AS $$
DECLARE
    resolved_tenant_id uuid;
BEGIN
    resolved_tenant_id := app.current_tenant_id();
    IF resolved_tenant_id IS NULL THEN
        RAISE EXCEPTION 'app.current_tenant_id must be set with SET LOCAL before tenant data access'
            USING ERRCODE = '42501';
    END IF;
    RETURN resolved_tenant_id;
END;
$$;

CREATE OR REPLACE FUNCTION app.touch_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.updated_at := clock_timestamp();
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION app.reject_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only; % is forbidden', TG_TABLE_NAME, TG_OP
        USING ERRCODE = '55000';
END;
$$;

-- ---------------------------------------------------------------------------
-- Tenancy, repositories, and integration identity
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) > 0),
    kms_key_reference text NOT NULL CHECK (length(btrim(kms_key_reference)) > 0),
    plan_tier text NOT NULL DEFAULT 'ENTERPRISE'
        CHECK (plan_tier IN ('TRIAL', 'TEAM', 'BUSINESS', 'ENTERPRISE')),
    emergency_lockdown boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE TABLE IF NOT EXISTS repositories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('github', 'gitlab', 'bitbucket', 'azure_devops', 'forgejo')),
    external_id text NOT NULL,
    canonical_url text NOT NULL CHECK (canonical_url ~ '^https://'),
    default_branch text NOT NULL DEFAULT 'main' CHECK (length(btrim(default_branch)) > 0),
    asset_criticality numeric(3,2) NOT NULL DEFAULT 1.00
        CHECK (asset_criticality >= 0.40 AND asset_criticality <= 2.50),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (tenant_id, provider, external_id),
    UNIQUE (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS repositories_tenant_active_idx
    ON repositories (tenant_id, is_active) WHERE is_active;

CREATE TRIGGER tenants_touch_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TRIGGER repositories_touch_updated_at
    BEFORE UPDATE ON repositories
    FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

-- ---------------------------------------------------------------------------
-- Organizational Policy Hierarchy (Global -> Workspace -> Repository -> Branch)
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS policies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    tier text NOT NULL CHECK (tier IN ('GLOBAL', 'WORKSPACE', 'REPOSITORY', 'BRANCH')),
    scope_id text NOT NULL, -- UUID or branch pattern
    name text NOT NULL,
    review_profile text NOT NULL DEFAULT 'assertive' CHECK (review_profile IN ('assertive', 'chill')),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS idx_policies_lookup
    ON policies (tenant_id, tier, scope_id) WHERE is_active;

CREATE TRIGGER policies_touch_updated_at
    BEFORE UPDATE ON policies
    FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE IF NOT EXISTS policy_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_id uuid NOT NULL REFERENCES policies(id) ON DELETE CASCADE,
    rule_identifier text NOT NULL,
    severity_override text CHECK (severity_override IN ('INFORMATIONAL', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    is_blocking boolean NOT NULL DEFAULT true,
    parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX IF NOT EXISTS idx_policy_rules_policy_id
    ON policy_rules (policy_id, is_blocking);

-- ---------------------------------------------------------------------------
-- Reviews and exact pipeline-stage outcomes
-- ---------------------------------------------------------------------------

-- A partitioned table can only have a primary/unique key that includes every
-- partition key. Consumers retain scan_created_at alongside scan_id to form a
-- valid foreign key and to allow partition pruning during retention operations.
CREATE TABLE IF NOT EXISTS scans (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    repository_id uuid NOT NULL,
    commit_sha text NOT NULL CHECK (commit_sha ~ '^[0-9A-Fa-f]{7,128}$'),
    base_sha text CHECK (base_sha IS NULL OR base_sha ~ '^[0-9A-Fa-f]{7,128}$'),
    pull_request_number bigint CHECK (pull_request_number IS NULL OR pull_request_number > 0),
    trigger scan_trigger NOT NULL,
    state scan_state NOT NULL DEFAULT 'QUEUED',
    overall_outcome analysis_outcome,
    policy_snapshot_sha256 char(64) NOT NULL CHECK (policy_snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    risk_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(risk_snapshot) = 'object'),
    composite_risk_score numeric(8,4) NOT NULL DEFAULT 0 CHECK (composite_risk_score >= 0),
    requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id, created_at),
    FOREIGN KEY (tenant_id, repository_id)
        REFERENCES repositories (tenant_id, id) ON DELETE RESTRICT,
    CHECK (completed_at IS NULL OR completed_at >= requested_at),
    CHECK (
        (state = 'COMPLETED' AND overall_outcome IS NOT NULL AND completed_at IS NOT NULL)
        OR (state <> 'COMPLETED' AND overall_outcome IS NULL)
    )
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS scans_tenant_repository_created_idx
    ON scans (tenant_id, repository_id, created_at DESC);
CREATE INDEX IF NOT EXISTS scans_tenant_id_idx
    ON scans (tenant_id, id);

CREATE TABLE IF NOT EXISTS scan_stage_results (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    scan_created_at timestamptz NOT NULL,
    stage analysis_stage NOT NULL,
    outcome analysis_outcome NOT NULL,
    analyzer_version text NOT NULL CHECK (length(btrim(analyzer_version)) > 0),
    summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(summary) = 'object'),
    warning_count integer NOT NULL DEFAULT 0 CHECK (warning_count >= 0),
    error_code text,
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, scan_id, scan_created_at)
        REFERENCES scans (tenant_id, id, created_at) ON DELETE CASCADE,
    UNIQUE (tenant_id, scan_id, scan_created_at, stage),
    CHECK (completed_at IS NULL OR started_at IS NULL OR completed_at >= started_at),
    CHECK ((outcome = 'PASS_WITH_WARNINGS') = (warning_count > 0)),
    CHECK ((outcome = 'ERROR') = (error_code IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS scan_stage_results_scan_idx
    ON scan_stage_results (tenant_id, scan_id, scan_created_at, stage);

-- ---------------------------------------------------------------------------
-- Evidence, findings, and risk inputs
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS evidence_packets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    scan_created_at timestamptz NOT NULL,
    kind evidence_kind NOT NULL,
    canonical_payload jsonb NOT NULL CHECK (jsonb_typeof(canonical_payload) = 'object'),
    payload_sha256 char(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    merkle_root_sha256 char(64) NOT NULL CHECK (merkle_root_sha256 ~ '^[0-9a-f]{64}$'),
    collector_version text NOT NULL CHECK (length(btrim(collector_version)) > 0),
    source_commit_sha text NOT NULL CHECK (source_commit_sha ~ '^[0-9A-Fa-f]{7,128}$'),
    signature_key_id text,
    signature bytea,
    verified_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, scan_id, scan_created_at)
        REFERENCES scans (tenant_id, id, created_at) ON DELETE CASCADE,
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, id, scan_id, scan_created_at),
    CHECK ((signature_key_id IS NULL) = (signature IS NULL)),
    CHECK (canonical_payload <> '{}'::jsonb)
);

CREATE INDEX IF NOT EXISTS evidence_packets_scan_idx
    ON evidence_packets (tenant_id, scan_id, scan_created_at, kind);
CREATE INDEX IF NOT EXISTS evidence_packets_merkle_root_idx
    ON evidence_packets (tenant_id, merkle_root_sha256);

CREATE TABLE IF NOT EXISTS findings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    scan_created_at timestamptz NOT NULL,
    repository_id uuid NOT NULL,
    rule_id text NOT NULL CHECK (length(btrim(rule_id)) > 0),
    rule_category text NOT NULL CHECK (rule_category IN ('SAST', 'SCA', 'SECRETS', 'IAC', 'DAST', 'POLICY')),
    severity text NOT NULL CHECK (severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW', 'INFO')),
    status finding_status NOT NULL DEFAULT 'OBSERVED',
    title text NOT NULL CHECK (length(btrim(title)) > 0),
    file_path text,
    start_line integer CHECK (start_line IS NULL OR start_line > 0),
    end_line integer CHECK (end_line IS NULL OR end_line >= start_line),
    ast_path text,
    cvss_score numeric(3,1) CHECK (cvss_score IS NULL OR (cvss_score >= 0 AND cvss_score <= 10)),
    epss_probability numeric(7,6) CHECK (epss_probability IS NULL OR (epss_probability >= 0 AND epss_probability <= 1)),
    compensating_control_factor numeric(4,3) NOT NULL DEFAULT 1.000
        CHECK (compensating_control_factor > 0 AND compensating_control_factor <= 1),
    runtime_reachable boolean NOT NULL DEFAULT false,
    evidence_packet_id uuid,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, scan_id, scan_created_at)
        REFERENCES scans (tenant_id, id, created_at) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, repository_id)
        REFERENCES repositories (tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, evidence_packet_id, scan_id, scan_created_at)
        REFERENCES evidence_packets (tenant_id, id, scan_id, scan_created_at) ON DELETE RESTRICT,
    UNIQUE (tenant_id, id),
    CHECK (status <> 'CONFIRMED' OR evidence_packet_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS findings_tenant_repository_open_idx
    ON findings (tenant_id, repository_id, severity, created_at DESC)
    WHERE status IN ('OBSERVED', 'CONFIRMED');
CREATE INDEX IF NOT EXISTS findings_evidence_packet_idx
    ON findings (tenant_id, evidence_packet_id) WHERE evidence_packet_id IS NOT NULL;

CREATE TRIGGER findings_touch_updated_at
    BEFORE UPDATE ON findings
    FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

-- Evidence is a record of what the detector actually observed. Corrections are
-- represented by a new packet and corresponding finding state transition; the
-- original proof is never silently rewritten or erased.
CREATE TRIGGER evidence_packets_append_only
    BEFORE UPDATE OR DELETE ON evidence_packets
    FOR EACH ROW EXECUTE FUNCTION app.reject_mutation();

-- ---------------------------------------------------------------------------
-- Policy memory, remediation, attestations, and immutable agent actions
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS security_memory (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    repository_id uuid,
    subject_type text NOT NULL CHECK (subject_type IN ('FINDING', 'RULE', 'REPOSITORY', 'POLICY_EXCEPTION', 'REMEDIATION')),
    subject_id uuid,
    decision_type text NOT NULL CHECK (decision_type IN ('SUPPRESSION', 'VERIFIED_FIX', 'EXCEPTION', 'TRIAGE_CONTEXT')),
    justification text NOT NULL CHECK (length(btrim(justification)) > 0),
    evidence_reference text,
    embedding vector(1536) NOT NULL,
    approved_by_principal_id text NOT NULL CHECK (length(btrim(approved_by_principal_id)) > 0),
    approved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, repository_id)
        REFERENCES repositories (tenant_id, id) ON DELETE CASCADE,
    CHECK (expires_at IS NULL OR expires_at > approved_at)
);

CREATE INDEX IF NOT EXISTS security_memory_tenant_subject_idx
    ON security_memory (tenant_id, subject_type, subject_id, created_at DESC);
CREATE INDEX IF NOT EXISTS security_memory_active_idx
    ON security_memory (tenant_id, expires_at) WHERE expires_at IS NULL;
CREATE INDEX IF NOT EXISTS security_memory_embedding_hnsw_idx
    ON security_memory USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

CREATE TRIGGER security_memory_touch_updated_at
    BEFORE UPDATE ON security_memory
    FOR EACH ROW EXECUTE FUNCTION app.touch_updated_at();

CREATE TABLE IF NOT EXISTS remediation_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    finding_id uuid NOT NULL,
    state remediation_state NOT NULL DEFAULT 'REQUESTED',
    sandbox_tier text NOT NULL CHECK (sandbox_tier IN ('STANDARD_GVISOR', 'STRONG_FIRECRACKER')),
    proposed_patch_sha256 char(64) CHECK (proposed_patch_sha256 IS NULL OR proposed_patch_sha256 ~ '^[0-9a-f]{64}$'),
    verification_evidence_packet_id uuid,
    requested_by_principal_id text NOT NULL CHECK (length(btrim(requested_by_principal_id)) > 0),
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, finding_id)
        REFERENCES findings (tenant_id, id) ON DELETE CASCADE,
    FOREIGN KEY (tenant_id, verification_evidence_packet_id)
        REFERENCES evidence_packets (tenant_id, id) ON DELETE RESTRICT,
    CHECK ((state = 'VERIFIED') = (verification_evidence_packet_id IS NOT NULL)),
    CHECK (completed_at IS NULL OR completed_at >= created_at)
);

CREATE INDEX IF NOT EXISTS remediation_attempts_finding_idx
    ON remediation_attempts (tenant_id, finding_id, created_at DESC);

CREATE TABLE IF NOT EXISTS assurance_manifests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    scan_id uuid NOT NULL,
    scan_created_at timestamptz NOT NULL,
    in_toto_statement jsonb NOT NULL CHECK (jsonb_typeof(in_toto_statement) = 'object'),
    payload_sha256 char(64) NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    signing_key_id text NOT NULL CHECK (length(btrim(signing_key_id)) > 0),
    ed25519_signature bytea NOT NULL CHECK (octet_length(ed25519_signature) = 64),
    issued_by_principal_id text NOT NULL CHECK (length(btrim(issued_by_principal_id)) > 0),
    issued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY (tenant_id, scan_id, scan_created_at)
        REFERENCES scans (tenant_id, id, created_at) ON DELETE RESTRICT,
    UNIQUE (tenant_id, scan_id, scan_created_at, payload_sha256)
);

CREATE INDEX IF NOT EXISTS assurance_manifests_scan_idx
    ON assurance_manifests (tenant_id, scan_id, scan_created_at, issued_at DESC);

CREATE TABLE IF NOT EXISTS agent_action_ledger (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id text NOT NULL CHECK (length(btrim(agent_id)) > 0),
    action_tier agent_action_tier NOT NULL,
    action_name text NOT NULL CHECK (length(btrim(action_name)) > 0),
    request_hash_sha256 char(64) NOT NULL CHECK (request_hash_sha256 ~ '^[0-9a-f]{64}$'),
    input_manifest jsonb NOT NULL CHECK (jsonb_typeof(input_manifest) = 'object'),
    result_manifest jsonb NOT NULL CHECK (jsonb_typeof(result_manifest) = 'object'),
    human_authorization_id uuid,
    human_authorization_key_id text,
    human_authorization_signature bytea,
    previous_entry_hash_sha256 char(64),
    entry_hash_sha256 char(64) NOT NULL CHECK (entry_hash_sha256 ~ '^[0-9a-f]{64}$'),
    execution_status text NOT NULL CHECK (execution_status IN ('AUTHORIZED', 'SUCCEEDED', 'DENIED', 'FAILED')),
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (previous_entry_hash_sha256 IS NULL OR previous_entry_hash_sha256 ~ '^[0-9a-f]{64}$'),
    CHECK (
        action_tier <> 'TIER_4_MUTATE'
        OR (
            human_authorization_id IS NOT NULL
            AND human_authorization_key_id IS NOT NULL
            AND human_authorization_signature IS NOT NULL
            AND octet_length(human_authorization_signature) > 0
        )
    ),
    CHECK (
        action_tier = 'TIER_4_MUTATE'
        OR (
            human_authorization_id IS NULL
            AND human_authorization_key_id IS NULL
            AND human_authorization_signature IS NULL
        )
    )
);

CREATE INDEX IF NOT EXISTS agent_action_ledger_tenant_time_idx
    ON agent_action_ledger (tenant_id, occurred_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS agent_action_ledger_entry_hash_idx
    ON agent_action_ledger (entry_hash_sha256);

CREATE TRIGGER agent_action_ledger_append_only
    BEFORE UPDATE OR DELETE ON agent_action_ledger
    FOR EACH ROW EXECUTE FUNCTION app.reject_mutation();

-- Transactional outbox: an integration event is committed with its domain change
-- and relayed later. This avoids acknowledging a webhook before durable intent.
CREATE TABLE IF NOT EXISTS integration_outbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    aggregate_type text NOT NULL CHECK (length(btrim(aggregate_type)) > 0),
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL CHECK (length(btrim(event_type)) > 0),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    idempotency_key text NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    published_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text,
    UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS integration_outbox_pending_idx
    ON integration_outbox (occurred_at, id)
    WHERE published_at IS NULL;

-- ---------------------------------------------------------------------------
-- RLS: force every tenant-owned relation through one transaction-local context.
-- ---------------------------------------------------------------------------

ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE repositories ENABLE ROW LEVEL SECURITY;
ALTER TABLE repositories FORCE ROW LEVEL SECURITY;
ALTER TABLE scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE scans FORCE ROW LEVEL SECURITY;
ALTER TABLE scan_stage_results ENABLE ROW LEVEL SECURITY;
ALTER TABLE scan_stage_results FORCE ROW LEVEL SECURITY;
ALTER TABLE evidence_packets ENABLE ROW LEVEL SECURITY;
ALTER TABLE evidence_packets FORCE ROW LEVEL SECURITY;
ALTER TABLE findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE findings FORCE ROW LEVEL SECURITY;
ALTER TABLE security_memory ENABLE ROW LEVEL SECURITY;
ALTER TABLE security_memory FORCE ROW LEVEL SECURITY;
ALTER TABLE remediation_attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE remediation_attempts FORCE ROW LEVEL SECURITY;
ALTER TABLE assurance_manifests ENABLE ROW LEVEL SECURITY;
ALTER TABLE assurance_manifests FORCE ROW LEVEL SECURITY;
ALTER TABLE agent_action_ledger ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_action_ledger FORCE ROW LEVEL SECURITY;
ALTER TABLE integration_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE integration_outbox FORCE ROW LEVEL SECURITY;

CREATE POLICY tenants_tenant_isolation ON tenants
    AS PERMISSIVE FOR ALL
    USING (id = app.current_tenant_id())
    WITH CHECK (id = app.current_tenant_id());

CREATE POLICY repositories_tenant_isolation ON repositories
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY scans_tenant_isolation ON scans
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY scan_stage_results_tenant_isolation ON scan_stage_results
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY evidence_packets_tenant_isolation ON evidence_packets
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY findings_tenant_isolation ON findings
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY security_memory_tenant_isolation ON security_memory
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY remediation_attempts_tenant_isolation ON remediation_attempts
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY assurance_manifests_tenant_isolation ON assurance_manifests
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY agent_action_ledger_tenant_isolation ON agent_action_ledger
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY integration_outbox_tenant_isolation ON integration_outbox
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

ALTER TABLE policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE policies FORCE ROW LEVEL SECURITY;
ALTER TABLE policy_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_rules FORCE ROW LEVEL SECURITY;

CREATE POLICY policies_tenant_isolation ON policies
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

CREATE POLICY policy_rules_tenant_isolation ON policy_rules
    AS PERMISSIVE FOR ALL
    USING (tenant_id = app.current_tenant_id())
    WITH CHECK (tenant_id = app.current_tenant_id());

-- Creates a monthly scans partition and protects the partition itself from
-- direct access. It is called only by the migration/partition-maintenance role.
CREATE OR REPLACE FUNCTION app.ensure_scans_partition(partition_month timestamptz)
RETURNS void
LANGUAGE plpgsql
SET search_path = public, app, pg_catalog
AS $$
DECLARE
    range_start timestamptz := date_trunc('month', partition_month AT TIME ZONE 'UTC') AT TIME ZONE 'UTC';
    range_end timestamptz := range_start + interval '1 month';
    partition_name text := format('scans_%s', to_char(range_start AT TIME ZONE 'UTC', 'YYYY_MM'));
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS public.%I PARTITION OF public.scans FOR VALUES FROM (%L) TO (%L)',
        partition_name, range_start, range_end
    );
    EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', partition_name);
    EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', partition_name);

    IF NOT EXISTS (
        SELECT 1
        FROM pg_policies
        WHERE schemaname = 'public'
          AND tablename = partition_name
          AND policyname = 'scans_partition_tenant_isolation'
    ) THEN
        EXECUTE format(
            'CREATE POLICY scans_partition_tenant_isolation ON public.%I AS PERMISSIVE FOR ALL '
            || 'USING (tenant_id = app.current_tenant_id()) '
            || 'WITH CHECK (tenant_id = app.current_tenant_id())',
            partition_name
        );
    END IF;
END;
$$;

-- Keep the current month and eighteen future months present. A deployment cron
-- invokes this same function monthly before the horizon is reached.
DO $$
DECLARE
    offset_month integer;
BEGIN
    FOR offset_month IN 0..18 LOOP
        PERFORM app.ensure_scans_partition(
            date_trunc('month', clock_timestamp()) + make_interval(months => offset_month)
        );
    END LOOP;
END;
$$;

-- Do not expose partition DDL to the runtime role. Replace scandrix_app with
-- the provisioned non-owner application role in each environment.
REVOKE ALL ON FUNCTION app.ensure_scans_partition(timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app.current_tenant_id() TO PUBLIC;
GRANT EXECUTE ON FUNCTION app.require_tenant_context() TO PUBLIC;

COMMIT;
```

## 3. Request transaction contract

RLS depends on a verified identity, not a client-supplied header. After validating the JWT, team key, or workload credential, the Go transaction helper binds the organization identifier for the duration of the transaction only:

```sql
BEGIN;
SELECT set_config('app.current_tenant_id', $1::text, true);
SELECT app.require_tenant_context();

-- Every tenant-owned query executes here. Do not issue COMMIT between the
-- context binding and the business operation.
SELECT id, state, overall_outcome
FROM scans
WHERE tenant_id = app.current_tenant_id() AND id = $2;

COMMIT;
```

`true` is essential: it gives `set_config` the same transaction-local behavior as `SET LOCAL`, so a pooled connection cannot retain a prior request's tenant context. The application must begin a transaction before binding context; an `AUTOCOMMIT` query is not a valid tenant-data access path.

## 4. Partition operations and retention

The deployment scheduler runs the following as the migration role on the first day of each month. It preserves an eighteen-month write horizon without introducing a permissive default partition.

```sql
SELECT app.ensure_scans_partition(date_trunc('month', clock_timestamp()) + interval '18 months');
```

Before detaching a historical partition, export immutable evidence and assurance records according to the tenant's retention policy, verify their hashes, and ensure no legal hold applies. Because `findings` and `evidence_packets` retain foreign keys to the scan partition, the retention job must archive or delete dependent records in the same governed workflow; it must never drop a scan partition opportunistically.

## 5. Database acceptance checks

Run these checks in CI against a disposable PostgreSQL 16 + pgvector instance after applying the migration:

```sql
-- Vector dimensionality and HNSW options are exact.
SELECT a.atttypmod
FROM pg_attribute AS a
WHERE a.attrelid = 'security_memory'::regclass
  AND a.attname = 'embedding';

SELECT indexdef
FROM pg_indexes
WHERE tablename = 'security_memory'
  AND indexname = 'security_memory_embedding_hnsw_idx';

-- The six outcomes are exhaustive and spelling-safe.
SELECT enumlabel
FROM pg_enum
WHERE enumtypid = 'analysis_outcome'::regtype
ORDER BY enumsortorder;

-- Every tenant-owned table has RLS enabled and forced.
SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
FROM pg_class AS c
WHERE c.relkind IN ('r', 'p')
  AND c.relname IN (
      'tenants', 'repositories', 'scans', 'scan_stage_results', 'evidence_packets',
      'findings', 'security_memory', 'remediation_attempts', 'assurance_manifests',
      'agent_action_ledger', 'integration_outbox'
  )
ORDER BY c.relname;

-- A confirmed finding cannot exist without deterministic evidence.
SELECT count(*) AS invalid_confirmed_findings
FROM findings
WHERE status = 'CONFIRMED' AND evidence_packet_id IS NULL;
```

Expected results are `1536` for the vector type modifier after pgvector's standard adjustment, an HNSW `vector_cosine_ops` index with `m = 16, ef_construction = 64`, the six listed outcomes in order, RLS flags set to true, and zero invalid confirmed findings. Application integration tests must additionally prove that a transaction with no tenant context returns no rows, rejects writes, and cannot read another tenant after connection reuse.
