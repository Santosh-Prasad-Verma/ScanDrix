-- Migration 017: Drixy Vector Memory Table for Rules & Feedback Learning
-- Creates drixy_embedding_vectors table and indexes with Row Level Security.

CREATE TABLE IF NOT EXISTS drixy_embedding_vectors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    entity_type VARCHAR(64) NOT NULL,
    entity_id VARCHAR(255) NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    embedding vector(1536),
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_ws ON drixy_embedding_vectors(workspace_id, entity_type);
CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_hash ON drixy_embedding_vectors(content_hash);

ALTER TABLE drixy_embedding_vectors ENABLE ROW LEVEL SECURITY;
ALTER TABLE drixy_embedding_vectors FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_drixy_embeddings ON drixy_embedding_vectors;
CREATE POLICY tenant_isolation_drixy_embeddings ON drixy_embedding_vectors
    FOR ALL
    USING (workspace_id = NULLIF(current_setting('app.current_workspace_id', true), '')::uuid);

COMMENT ON TABLE drixy_embedding_vectors IS 'Drixy vector memory for rules & feedback learning';
