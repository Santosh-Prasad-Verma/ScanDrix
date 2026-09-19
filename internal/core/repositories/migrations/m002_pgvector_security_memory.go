package migrations

import (
	"context"
)

// Migration002PgvectorSecurityMemory represents database migration 002_pgvector_security_memory.
type Migration002PgvectorSecurityMemory struct{}

// Version returns the unique migration version string.
func (m *Migration002PgvectorSecurityMemory) Version() string {
	return "002"
}

// Name returns the descriptive name of the migration.
func (m *Migration002PgvectorSecurityMemory) Name() string {
	return "002_pgvector_security_memory"
}

// Up applies the schema changes defined in 002_pgvector_security_memory.sql.
func (m *Migration002PgvectorSecurityMemory) Up(ctx context.Context, exec SQLExecutor) error {
	query := `-- Migration 002: pgvector Security Memory & Semantic HNSW Vector Retrieval
-- Backs historical false-positive suppression and semantic code review memory

CREATE EXTENSION IF NOT EXISTS vector;

-- Security Memory Vector Embeddings (1536-dimensional OpenAI / Voyage / Gemini vectors)
CREATE TABLE IF NOT EXISTS security_memory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    finding_fingerprint VARCHAR(128) NOT NULL,
    category VARCHAR(64) NOT NULL,
    rule_id VARCHAR(128) NOT NULL,
    code_snippet TEXT NOT NULL,
    justification TEXT NOT NULL,
    dismissal_reason VARCHAR(64) NOT NULL DEFAULT 'FALSE_POSITIVE',
    embedding vector(1536) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fast Hierarchical Navigable Small World (HNSW) Cosine Distance Index
CREATE INDEX IF NOT EXISTS idx_security_memory_hnsw 
ON security_memory USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

CREATE INDEX IF NOT EXISTS idx_security_memory_workspace ON security_memory(workspace_id);
CREATE INDEX IF NOT EXISTS idx_security_memory_fingerprint ON security_memory(workspace_id, finding_fingerprint);

-- Row-Level Security Enforcement for Vector Memory
ALTER TABLE security_memory ENABLE ROW LEVEL SECURITY;
ALTER TABLE security_memory FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_security_memory ON security_memory;
CREATE POLICY tenant_isolation_security_memory ON security_memory
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid);`
	_, err := exec.ExecContext(ctx, query)
	return err
}

// Down reverts the schema changes defined in 002_pgvector_security_memory.sql.
func (m *Migration002PgvectorSecurityMemory) Down(ctx context.Context, exec SQLExecutor) error {
	query := `DROP TABLE IF EXISTS security_memory CASCADE;`
	_, err := exec.ExecContext(ctx, query)
	return err
}
