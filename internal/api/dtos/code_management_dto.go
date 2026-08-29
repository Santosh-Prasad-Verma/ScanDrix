package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// TrackRepositoryRequest requests adding a Git repository to automated review monitoring.
type TrackRepositoryRequest struct {
	Provider      models.SCMProvider `json:"provider"`
	ExternalID    string             `json:"external_id"`
	NamespacePath string             `json:"namespace_path"`
	DefaultBranch string             `json:"default_branch"`
}

// RepositoryResponse returns tracked repository status and settings.
type RepositoryResponse struct {
	ID            uuid.UUID          `json:"id"`
	WorkspaceID   uuid.UUID          `json:"workspace_id"`
	Provider      models.SCMProvider `json:"provider"`
	ExternalID    string             `json:"external_id"`
	NamespacePath string             `json:"namespace_path"`
	DefaultBranch string             `json:"default_branch"`
	IsActive      bool               `json:"is_active"`
	CreatedAt     time.Time          `json:"created_at"`
}

// BranchListResponse lists active Git branches.
type BranchListResponse struct {
	DefaultBranch string   `json:"default_branch"`
	Branches      []string `json:"branches"`
}

// FileTreeEntry represents a file or directory inside a repository tree.
type FileTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
	Size int64  `json:"size,omitempty"`
}

// FileTreeResponse returns repository directory contents.
type FileTreeResponse struct {
	CommitSHA string          `json:"commit_sha"`
	Entries   []FileTreeEntry `json:"entries"`
}
