// Package database provides PostgreSQL AST graph storage and caller analysis.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
)

// BatchInsertASTNodes persists extracted AST code symbols into the code_ast_nodes table (Migration 003).
func (r *Repository) BatchInsertASTNodes(ctx context.Context, repoID uuid.UUID, nodes []graph.ASTNode) error {
	if r == nil || r.client == nil || r.client.Pool == nil || len(nodes) == 0 {
		return nil
	}

	query := `
		INSERT INTO code_ast_nodes (
			id, repository_id, kind, symbol_name, file_path, start_line, end_line, signature, language, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			symbol_name = EXCLUDED.symbol_name,
			file_path = EXCLUDED.file_path,
			start_line = EXCLUDED.start_line,
			end_line = EXCLUDED.end_line,
			signature = EXCLUDED.signature,
			updated_at = EXCLUDED.updated_at;
	`

	batch := &pgx.Batch{}
	for _, n := range nodes {
		id := n.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		updatedAt := n.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}
		batch.Queue(query, id, repoID, string(n.Kind), n.SymbolName, n.FilePath, n.StartLine, n.EndLine, n.Signature, n.Language, updatedAt)
	}

	br := r.client.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range nodes {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to insert AST node batch: %w", err)
		}
	}
	return nil
}


// BatchInsertASTEdges persists architectural call and import graphs into code_ast_edges (Migration 003).
func (r *Repository) BatchInsertASTEdges(ctx context.Context, repoID uuid.UUID, edges []graph.ASTEdge) error {
	if r == nil || r.client == nil || r.client.Pool == nil || len(edges) == 0 {
		return nil
	}

	query := `
		INSERT INTO code_ast_edges (id, repository_id, from_node_id, to_node_id, kind)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING;
	`

	batch := &pgx.Batch{}
	for _, e := range edges {
		id := e.ID
		if id == uuid.Nil {
			id = uuid.New()
		}
		batch.Queue(query, id, repoID, e.FromNodeID, e.ToNodeID, string(e.Kind))
	}

	br := r.client.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range edges {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to insert AST edge batch: %w", err)
		}
	}
	return nil
}


// GetASTNodesByRepository retrieves indexed code symbols for a repository.
func (r *Repository) GetASTNodesByRepository(ctx context.Context, repoID uuid.UUID) ([]graph.ASTNode, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT id, repository_id, kind, symbol_name, file_path, start_line, end_line, signature, language, updated_at
		FROM code_ast_nodes
		WHERE repository_id = $1
		ORDER BY file_path, start_line;
	`

	rows, err := r.client.Pool.Query(ctx, query, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed querying AST nodes: %w", err)
	}
	defer rows.Close()

	var nodes []graph.ASTNode
	for rows.Next() {
		var n graph.ASTNode
		var kind string
		if err := rows.Scan(
			&n.ID, &n.RepositoryID, &kind, &n.SymbolName, &n.FilePath,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Language, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.Kind = graph.ASTNodeKind(kind)
		nodes = append(nodes, n)
	}
	return nodes, nil
}


// GetASTCallers traverses code_ast_edges to locate functions calling targetNodeID.
func (r *Repository) GetASTCallers(ctx context.Context, repoID, targetNodeID uuid.UUID) ([]graph.ASTNode, error) {
	if r == nil || r.client == nil || r.client.Pool == nil {
		return nil, errors.New("database repository unavailable")
	}

	query := `
		SELECT n.id, n.repository_id, n.kind, n.symbol_name, n.file_path, n.start_line, n.end_line, n.signature, n.language, n.updated_at
		FROM code_ast_nodes n
		INNER JOIN code_ast_edges e ON e.from_node_id = n.id
		WHERE e.repository_id = $1 AND e.to_node_id = $2 AND e.kind = 'CALLS'
		ORDER BY n.file_path, n.start_line;
	`

	rows, err := r.client.Pool.Query(ctx, query, repoID, targetNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed querying AST callers: %w", err)
	}
	defer rows.Close()

	var callers []graph.ASTNode
	for rows.Next() {
		var n graph.ASTNode
		var kind string
		if err := rows.Scan(
			&n.ID, &n.RepositoryID, &kind, &n.SymbolName, &n.FilePath,
			&n.StartLine, &n.EndLine, &n.Signature, &n.Language, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.Kind = graph.ASTNodeKind(kind)
		callers = append(callers, n)
	}
	return callers, nil
}

