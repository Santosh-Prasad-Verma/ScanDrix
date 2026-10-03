-- Migration 019: Fix Drixy Embeddings RLS Session Variable, Null Safety and HNSW Index
-- Resolves uuid syntax error on empty session variable and adds HNSW cosine index for pgvector.
--
-- SAFETY (AUDIT_REMEDIATION.md F-33)
-- The original version of this migration dropped and re-added `embedding`
-- whenever the column was typed ARRAY, and nothing checked whether the column
-- held data. On a populated installation that silently and irreversibly
-- destroyed every accumulated vector: the vectors are the entire value of this
-- table, they are not reproducible from source, and semantic search would then
-- silently return nothing while reporting success (AGENTS.md 2.7.2).
--
-- This version refuses to proceed when there is anything to lose. It will not
-- decide on an operator's behalf that their embeddings are disposable.
--
-- To resolve a genuinely populated table, back it up first, then convert
-- explicitly and knowingly, e.g.:
--
--   CREATE TABLE drixy_embedding_vectors_backup AS SELECT * FROM drixy_embedding_vectors;
--   -- inspect, then either convert in place or re-embed from source
--
-- An operator who has confirmed the table is empty or has a backup can set
-- ALLOW_EMBEDDING_COLUMN_DROP=true for the duration of the migration.

DO $$
DECLARE
    vec_rows BIGINT;
    -- COALESCE is required, not stylistic. current_setting(..., true) returns
    -- NULL when the GUC was never set, so a bare comparison yields NULL, and
    -- `IF <NULL>` is not true -- the guard would be silently skipped and the
    -- destructive path would run anyway. That is the exact failure this
    -- migration is meant to prevent.
    drop_allowed BOOLEAN := COALESCE(
        current_setting('app.allow_embedding_column_drop', true), 'false'
    ) = 'true';
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'public' AND table_name = 'drixy_embedding_vectors'
    ) THEN
        -- Ensure the pgvector extension is present. Without it the ALTER below
        -- fails later with a confusing cast error instead of a clear message.
        IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
            RAISE EXCEPTION
                'Migration 019 requires the pgvector "vector" extension, which is not installed. '
                'Run CREATE EXTENSION vector; before retrying.';
        END IF;

        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = 'public' AND table_name = 'drixy_embedding_vectors'
              AND column_name = 'embedding'
              AND data_type = 'ARRAY'
        ) THEN
            -- Refuse to destroy data. See the header for the escape hatch.
            EXECUTE 'SELECT count(*) FROM drixy_embedding_vectors WHERE embedding IS NOT NULL'
                INTO vec_rows;

            IF vec_rows > 0 AND NOT drop_allowed THEN
                RAISE EXCEPTION
                    'Migration 019 refused to drop drixy_embedding_vectors.embedding: % row(s) hold data. '
                    'This column stores generated embeddings that cannot be recovered from source, and '
                    'dropping it is irreversible. Back the table up, convert or re-embed the vectors '
                    'explicitly, then re-run. To override deliberately, set '
                    'app.allow_embedding_column_drop = ''true'' for this session.',
                    vec_rows
                    USING HINT = 'SELECT count(*) FROM drixy_embedding_vectors WHERE embedding IS NOT NULL;';
            END IF;

            IF vec_rows > 0 THEN
                RAISE WARNING
                    'Migration 019 is DROPPING drixy_embedding_vectors.embedding with % row(s) of data '
                    'because app.allow_embedding_column_drop was set. This is irreversible.',
                    vec_rows;
            END IF;

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
        -- Wrap in a guard: if the vector extension was only just installed into a
        -- different schema this fails, and it must not abort the whole migration.
        BEGIN
            CREATE INDEX IF NOT EXISTS idx_drixy_embeddings_hnsw
                ON drixy_embedding_vectors USING hnsw (embedding vector_cosine_ops);
        EXCEPTION
            WHEN OTHERS THEN
                RAISE WARNING 'Skipping HNSW index on drixy_embedding_vectors: %', SQLERRM;
        END;
    END IF;
END $$;
