package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/core/domain"
)

// PgSuggestionEmbeddingRepository implements domain.SuggestionEmbeddingRepository using pgvector.
type PgSuggestionEmbeddingRepository struct {
	pool *pgxpool.Pool
}

// NewSuggestionEmbeddingRepository instantiates a new PgSuggestionEmbeddingRepository.
func NewSuggestionEmbeddingRepository(pool *pgxpool.Pool) *PgSuggestionEmbeddingRepository {
	return &PgSuggestionEmbeddingRepository{pool: pool}
}

// Store persists a suggestion embedding vector.
func (r *PgSuggestionEmbeddingRepository) Store(ctx context.Context, emb *domain.SuggestionEmbedding) error {
	query := `
		INSERT INTO drixy_suggestion_embeddings (
			id, created_at, updated_at, workspace_id, repository_id, finding_id,
			content_hash, text_content, embedding, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (workspace_id, content_hash) DO UPDATE
		SET updated_at = EXCLUDED.updated_at,
		    embedding = EXCLUDED.embedding,
		    metadata = EXCLUDED.metadata
	`
	now := time.Now().UTC()
	if emb.ID == uuid.Nil {
		emb.ID = uuid.New()
	}
	emb.CreatedAt = now
	emb.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		emb.ID, emb.CreatedAt, emb.UpdatedAt, emb.WorkspaceID, emb.RepositoryID, emb.FindingID,
		emb.ContentHash, emb.TextContent, emb.Embedding, emb.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed storing suggestion embedding: %w", err)
	}
	return nil
}

// FindSimilar runs an HNSW cosine distance vector similarity query (`<=>`).
func (r *PgSuggestionEmbeddingRepository) FindSimilar(
	ctx context.Context,
	wsID, repoID uuid.UUID,
	vector domain.FloatVector,
	limit int,
	threshold float32,
) ([]*domain.SuggestionEmbedding, error) {
	if limit <= 0 {
		limit = 5
	}
	// Distance operator: 1 - (embedding <=> $3) gives cosine similarity
	query := `
		SELECT id, created_at, updated_at, workspace_id, repository_id, finding_id,
		       content_hash, text_content, metadata,
		       (1 - (embedding <=> $3)) AS similarity
		FROM drixy_suggestion_embeddings
		WHERE workspace_id = $1 AND repository_id = $2
		  AND (1 - (embedding <=> $3)) >= $4
		ORDER BY embedding <=> $3 ASC
		LIMIT $5
	`
	rows, err := r.pool.Query(ctx, query, wsID, repoID, vector, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("failed executing vector similarity search: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.SuggestionEmbedding, 0, limit)
	for rows.Next() {
		emb := &domain.SuggestionEmbedding{}
		var similarity float32
		if err := rows.Scan(
			&emb.ID, &emb.CreatedAt, &emb.UpdatedAt, &emb.WorkspaceID, &emb.RepositoryID, &emb.FindingID,
			&emb.ContentHash, &emb.TextContent, &emb.Metadata, &similarity,
		); err != nil {
			return nil, fmt.Errorf("failed scanning vector row: %w", err)
		}
		if emb.Metadata == nil {
			emb.Metadata = make(domain.JSONBMap)
		}
		emb.Metadata["similarity"] = similarity
		items = append(items, emb)
	}
	return items, nil
}

// PgAstGraphRepository persists AST nodes and edges for cross-file dependency graph analysis.
type PgAstGraphRepository struct {
	pool *pgxpool.Pool
}

// NewAstGraphRepository instantiates a new PgAstGraphRepository.
func NewAstGraphRepository(pool *pgxpool.Pool) *PgAstGraphRepository {
	return &PgAstGraphRepository{pool: pool}
}

// StoreNodesBatch performs high-throughput batch insertion of parsed AST nodes.
func (r *PgAstGraphRepository) StoreNodesBatch(ctx context.Context, nodes []*domain.AstNode) error {
	if len(nodes) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}

	query := `
		INSERT INTO ast_nodes (
			id, created_at, updated_at, workspace_id, repository_id, commit_sha,
			file_path, node_type, identifier, package_name, line_start, line_end,
			signature, properties
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (workspace_id, repository_id, commit_sha, file_path, identifier) DO NOTHING
	`
	for _, n := range nodes {
		if n.ID == uuid.Nil {
			n.ID = uuid.New()
		}
		n.CreatedAt = now
		n.UpdatedAt = now

		batch.Queue(query,
			n.ID, n.CreatedAt, n.UpdatedAt, n.WorkspaceID, n.RepositoryID, n.CommitSHA,
			n.FilePath, n.NodeType, n.Identifier, n.PackageName, n.LineStart, n.LineEnd,
			n.Signature, n.Properties,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range nodes {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert ast node failed: %w", err)
		}
	}
	return nil
}

// StoreEdgesBatch persists call graph edges and type inheritance links.
func (r *PgAstGraphRepository) StoreEdgesBatch(ctx context.Context, edges []*domain.AstEdge) error {
	if len(edges) == 0 {
		return nil
	}
	now := time.Now().UTC()
	batch := &pgx.Batch{}

	query := `
		INSERT INTO ast_edges (
			id, created_at, updated_at, workspace_id, repository_id,
			source_node_id, target_node_id, edge_type, weight, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (workspace_id, source_node_id, target_node_id, edge_type) DO NOTHING
	`
	for _, e := range edges {
		if e.ID == uuid.Nil {
			e.ID = uuid.New()
		}
		e.CreatedAt = now
		e.UpdatedAt = now

		batch.Queue(query,
			e.ID, e.CreatedAt, e.UpdatedAt, e.WorkspaceID, e.RepositoryID,
			e.SourceNodeID, e.TargetNodeID, e.EdgeType, e.Weight, e.Metadata,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range edges {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch insert ast edge failed: %w", err)
		}
	}
	return nil
}

// QueryNeighbors traverses call graph connections to return affected dependencies.
func (r *PgAstGraphRepository) QueryNeighbors(ctx context.Context, wsID, nodeID uuid.UUID) ([]*domain.AstNode, error) {
	query := `
		SELECT n.id, n.created_at, n.updated_at, n.workspace_id, n.repository_id, n.commit_sha,
		       n.file_path, n.node_type, n.identifier, n.package_name, n.line_start, n.line_end,
		       n.signature, n.properties
		FROM ast_nodes n
		INNER JOIN ast_edges e ON e.target_node_id = n.id
		WHERE e.workspace_id = $1 AND e.source_node_id = $2
	`
	rows, err := r.pool.Query(ctx, query, wsID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed querying ast neighbors: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.AstNode, 0)
	for rows.Next() {
		n := &domain.AstNode{}
		if err := rows.Scan(
			&n.ID, &n.CreatedAt, &n.UpdatedAt, &n.WorkspaceID, &n.RepositoryID, &n.CommitSHA,
			&n.FilePath, &n.NodeType, &n.Identifier, &n.PackageName, &n.LineStart, &n.LineEnd,
			&n.Signature, &n.Properties,
		); err != nil {
			return nil, fmt.Errorf("failed scanning ast neighbor node: %w", err)
		}
		items = append(items, n)
	}
	return items, nil
}

// PgContextReferenceRepository persists ticket links from Jira, Linear, and Azure Boards.
type PgContextReferenceRepository struct {
	pool *pgxpool.Pool
}

// NewContextReferenceRepository instantiates a new PgContextReferenceRepository.
func NewContextReferenceRepository(pool *pgxpool.Pool) *PgContextReferenceRepository {
	return &PgContextReferenceRepository{pool: pool}
}

// Create records an external issue ticket linked to a review run.
func (r *PgContextReferenceRepository) Create(ctx context.Context, cr *domain.ContextReference) error {
	query := `
		INSERT INTO context_references (
			id, created_at, updated_at, workspace_id, review_id, provider,
			reference_id, reference_url, title, summary, status, raw_payload
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	now := time.Now().UTC()
	if cr.ID == uuid.Nil {
		cr.ID = uuid.New()
	}
	cr.CreatedAt = now
	cr.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		cr.ID, cr.CreatedAt, cr.UpdatedAt, cr.WorkspaceID, cr.ReviewID, cr.Provider,
		cr.ReferenceID, cr.ReferenceURL, cr.Title, cr.Summary, cr.Status, cr.RawPayload,
	)
	if err != nil {
		return fmt.Errorf("failed creating context reference: %w", err)
	}
	return nil
}

// ListByReview retrieves all attached context references for a review execution.
func (r *PgContextReferenceRepository) ListByReview(ctx context.Context, wsID, reviewID uuid.UUID) ([]*domain.ContextReference, error) {
	query := `
		SELECT id, created_at, updated_at, workspace_id, review_id, provider,
		       reference_id, reference_url, title, summary, status, raw_payload
		FROM context_references
		WHERE workspace_id = $1 AND review_id = $2
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, wsID, reviewID)
	if err != nil {
		return nil, fmt.Errorf("failed querying context references: %w", err)
	}
	defer rows.Close()

	items := make([]*domain.ContextReference, 0)
	for rows.Next() {
		cr := &domain.ContextReference{}
		if err := rows.Scan(
			&cr.ID, &cr.CreatedAt, &cr.UpdatedAt, &cr.WorkspaceID, &cr.ReviewID, &cr.Provider,
			&cr.ReferenceID, &cr.ReferenceURL, &cr.Title, &cr.Summary, &cr.Status, &cr.RawPayload,
		); err != nil {
			return nil, fmt.Errorf("failed scanning context reference row: %w", err)
		}
		items = append(items, cr)
	}
	return items, nil
}
