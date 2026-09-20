-- Migration 019: Fix Drixy Embeddings RLS Session Variable, Null Safety and HNSW Index
-- Resolves uuid syntax error on empty session variable and adds HNSW cosine index for pgvector.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'drixy_embedding_vectors'
    ) THEN
        -- Ensure embedding column is of type vector(1536) for pgvector HNSW indexing
        IF EXISTS (
            SELECT 1 FROM information_schema.columns 
            WHERE table_schema = 'public' AND table_name = 'drixy_embedding_vectors' 
              AND column_name = 'embedding' 
              AND data_type = 'ARRAY'
        ) THEN
            ALTER TABLE drixy_embedding_vectors DROP COLUMN embedding;
            ALTER TABLE drixy_embedding_vectors ADD COLUMN embedding vector(1536);
        END IF;

        -- Drop fragile existing policy
        DROP POLICY IF EXISTS tenant_isolation_drixy_embeddings ON drixy_embedding_vectors;

        -- Create robust policy with NULLIF and dual-setting compatibility
        CREATE POLICY tenant_isolation_drixy_embeddings ON drixy_embedding_vectors
            FOR ALL
            USING (
                current_setting('app.is_system_worker', true) = 'true'
                OR workspace_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
                OR workspace_id = NULLIF(current_setting('app.current_workspace_id', true), '')::uuid
            );

        -- Force RLS to prevent table owner bypass
        ALTER TABLE drixy_embedding_vectors ENABLE ROW LEVEL SECURITY;
        ALTER TABLE drixy_embedding_vectors FORCE ROW LEVEL SECURITY;

        -- Create HNSW cosine vector index for fast semantic search (prevents sequential table scan)
        CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_hnsw 
            ON drixy_embedding_vectors USING hnsw (embedding vector_cosine_ops);
    END IF;
END $$;
