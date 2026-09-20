package sandbox

import (
	"time"

	"github.com/google/uuid"
)

// WorktreeInstance represents an isolated checkout created for a code review task.
type WorktreeInstance struct {
	ID            uuid.UUID `json:"id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	RepoNamespace string    `json:"repo_namespace"`
	CommitSHA     string    `json:"commit_sha"`
	WorktreePath  string    `json:"worktree_path"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	CleanedUp     bool      `json:"cleaned_up"`
}

// SandboxConfig specifies filesystem isolation and TTL boundaries.
type SandboxConfig struct {
	BaseDir       string        `json:"base_dir"`
	MaxFileSizeMB int64         `json:"max_file_size_mb"` // Max allowed file read/write (e.g. 20MB)
	DefaultTTL    time.Duration `json:"default_ttl"`      // e.g. 15 minutes
	AutoCleanup   bool          `json:"auto_cleanup"`
}

// SecurityViolation records an attempted sandbox breakout.
type SecurityViolation struct {
	AttemptedPath string    `json:"attempted_path"`
	SandboxRoot   string    `json:"sandbox_root"`
	Reason        string    `json:"reason"`
	Timestamp     time.Time `json:"timestamp"`
}
