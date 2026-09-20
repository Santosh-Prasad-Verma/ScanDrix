package domain

import (
	"github.com/google/uuid"
)

// AstNode models code syntax nodes for cross-file dependency graph analysis.
type AstNode struct {
	TenantScopedEntity
	RepositoryID uuid.UUID `json:"repository_id" db:"repository_id"`
	CommitSHA    string    `json:"commit_sha" db:"commit_sha"`
	FilePath     string    `json:"file_path" db:"file_path"`
	NodeType     string    `json:"node_type" db:"node_type"`
	Identifier   string    `json:"identifier" db:"identifier"`
	PackageName  string    `json:"package_name" db:"package_name"`
	LineStart    int       `json:"line_start" db:"line_start"`
	LineEnd      int       `json:"line_end" db:"line_end"`
	Signature    *string   `json:"signature,omitempty" db:"signature"`
	Properties   JSONBMap  `json:"properties" db:"properties"`
}

// AstNodeModel alias.
type AstNodeModel = AstNode

// AstEdge models call graph edges, inheritance, and import relationships between AST nodes.
type AstEdge struct {
	TenantScopedEntity
	RepositoryID uuid.UUID `json:"repository_id" db:"repository_id"`
	SourceNodeID uuid.UUID `json:"source_node_id" db:"source_node_id"`
	TargetNodeID uuid.UUID `json:"target_node_id" db:"target_node_id"`
	EdgeType     string    `json:"edge_type" db:"edge_type"`
	Weight       float32   `json:"weight" db:"weight"`
	Metadata     JSONBMap  `json:"metadata" db:"metadata"`
}

// AstEdgeModel alias.
type AstEdgeModel = AstEdge

// ContextReference models external Jira/Linear tickets or architecture docs attached to a review.
type ContextReference struct {
	TenantScopedEntity
	ReviewID     uuid.UUID `json:"review_id" db:"review_id"`
	Provider     string    `json:"provider" db:"provider"`
	ReferenceID  string    `json:"reference_id" db:"reference_id"`
	ReferenceURL string    `json:"reference_url" db:"reference_url"`
	Title        string    `json:"title" db:"title"`
	Summary      *string   `json:"summary,omitempty" db:"summary"`
	Status       string    `json:"status" db:"status"`
	RawPayload   JSONBMap  `json:"raw_payload" db:"raw_payload"`
}

// ContextReferenceModel alias.
type ContextReferenceModel = ContextReference
