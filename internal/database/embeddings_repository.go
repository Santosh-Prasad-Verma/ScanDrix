// Package database provides PostgreSQL pgvector embeddings repository.
package database

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SecurityMemoryRecord models semantic review findings in PostgreSQL pgvector memory.
type SecurityMemoryRecord struct {
	ID                 uuid.UUID `json:"id"`
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	FindingFingerprint string    `json:"finding_fingerprint"`
	Category           string    `json:"category"`
	RuleID             string    `json:"rule_id"`
	CodeSnippet        string    `json:"code_snippet"`
	Justification      string    `json:"justification"`
	DismissalReason    string    `json:"dismissal_reason"`
	SimilarityScore    float64   `json:"similarity_score,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}


// SaveSecurityMemory stores a semantic finding into pgvector security memory.
func (r *Repository) SaveSecurityMemory(ctx context.Context, wsID uuid.UUID, mem *SecurityMemoryRecord) error {
	return r.UpsertSecurityMemoryWithEmbedding(ctx, wsID, mem, nil)
}


// UpsertSecurityMemoryWithEmbedding stores or updates a security finding vector in pgvector memory.
func (r *Repository) UpsertSecurityMemoryWithEmbedding(ctx context.Context, wsID uuid.UUID, mem *SecurityMemoryRecord, embedding []float32) error {
	if r == nil || r.client == nil {
		return nil
	}
	if mem.ID == uuid.Nil {
		mem.ID = uuid.New()
	}
	mem.WorkspaceID = wsID
	mem.CreatedAt = time.Now().UTC()

	var query string
	var args []any

	if len(embedding) > 0 {
		vecStr := formatVector(embedding)
		query = `
			INSERT INTO security_memory (
				id, workspace_id, finding_fingerprint, category, rule_id,
				code_snippet, justification, dismissal_reason, embedding, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::vector, $10)
			ON CONFLICT (id) DO UPDATE SET
				code_snippet = EXCLUDED.code_snippet,
				justification = EXCLUDED.justification,
				dismissal_reason = EXCLUDED.dismissal_reason,
				embedding = EXCLUDED.embedding;
		`
		args = []any{
			mem.ID, mem.WorkspaceID, mem.FindingFingerprint, mem.Category, mem.RuleID,
			mem.CodeSnippet, mem.Justification, mem.DismissalReason, vecStr, mem.CreatedAt,
		}
	} else {
		query = `
			INSERT INTO security_memory (
				id, workspace_id, finding_fingerprint, category, rule_id,
				code_snippet, justification, dismissal_reason, embedding, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, array_fill(0.0::real, ARRAY[1536])::vector, $9)
			ON CONFLICT (id) DO NOTHING;
		`
		args = []any{
			mem.ID, mem.WorkspaceID, mem.FindingFingerprint, mem.Category, mem.RuleID,
			mem.CodeSnippet, mem.Justification, mem.DismissalReason, mem.CreatedAt,
		}
	}

	return r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, query, args...)
		return err
	})
}


// SearchSimilarSecurityFindings executes cosine distance semantic similarity vector search in pgvector.
func (r *Repository) SearchSimilarSecurityFindings(ctx context.Context, wsID uuid.UUID, embedding []float32, category string, limit int, maxDistance float64) ([]SecurityMemoryRecord, error) {
	if r == nil || r.client == nil || len(embedding) == 0 {
		return []SecurityMemoryRecord{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if maxDistance <= 0 {
		maxDistance = 0.5
	}
	vecStr := formatVector(embedding)
	query := `
		SELECT id, workspace_id, finding_fingerprint, category, rule_id,
		       code_snippet, justification, dismissal_reason,
		       (embedding <=> $2::vector) as distance, created_at
		FROM security_memory
		WHERE workspace_id = $1 AND ($3 = '' OR category = $3)
		  AND (embedding <=> $2::vector) <= $4
		ORDER BY distance ASC
		LIMIT $5;
	`
	var results []SecurityMemoryRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, wsID, vecStr, category, maxDistance, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m SecurityMemoryRecord
			var dist float64
			if err := rows.Scan(
				&m.ID, &m.WorkspaceID, &m.FindingFingerprint, &m.Category, &m.RuleID,
				&m.CodeSnippet, &m.Justification, &m.DismissalReason, &dist, &m.CreatedAt,
			); err != nil {
				return err
			}
			m.SimilarityScore = 1.0 - dist
			results = append(results, m)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed semantic security memory search: %w", err)
	}
	return results, nil
}


// GetSecurityMemoryByFingerprint checks if a finding has been recorded as a suppressed false positive.
func (r *Repository) GetSecurityMemoryByFingerprint(ctx context.Context, wsID uuid.UUID, fingerprint string) (*SecurityMemoryRecord, error) {
	if r == nil || r.client == nil {
		return nil, nil
	}

	var mem SecurityMemoryRecord
	err := r.client.ExecWithTenant(ctx, wsID, func(tx pgx.Tx) error {
		query := `
			SELECT id, workspace_id, finding_fingerprint, category, rule_id,
			       code_snippet, justification, dismissal_reason, created_at
			FROM security_memory
			WHERE workspace_id = $1 AND finding_fingerprint = $2
			LIMIT 1
		`
		return tx.QueryRow(ctx, query, wsID, fingerprint).Scan(
			&mem.ID, &mem.WorkspaceID, &mem.FindingFingerprint, &mem.Category, &mem.RuleID,
			&mem.CodeSnippet, &mem.Justification, &mem.DismissalReason, &mem.CreatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &mem, nil
}


func formatVector(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(f), 'f', 6, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}

