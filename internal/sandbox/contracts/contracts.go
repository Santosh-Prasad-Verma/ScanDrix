package contracts

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// SandboxTier identifies the isolation technology tier.
type SandboxTier string

const (
	TierWorktree SandboxTier = "WORKTREE"    // Fast local ephemeral checkout
	TierMicroVM  SandboxTier = "E2B_MICROVM" // Isolated remote container/VM
	TierNull     SandboxTier = "NULL"        // In-memory / mock for testing
)

// ProviderType identifies the configured sandbox provider backend.
type ProviderType string

const (
	ProviderAuto  ProviderType = "auto"
	ProviderE2B   ProviderType = "e2b"
	ProviderLocal ProviderType = "local"
	ProviderNull  ProviderType = "null"
)

// CreateSandboxParams defines the parameters required to initialize and clone a repository into a sandbox.
type CreateSandboxParams struct {
	CloneURL        string             `json:"clone_url"`
	AuthToken       string             `json:"auth_token,omitempty"`
	AuthUsername    string             `json:"auth_username,omitempty"`
	Branch          string             `json:"branch"`
	Platform        models.SCMProvider `json:"platform"`
	PRNumber        int                `json:"pr_number,omitempty"`
	BaseBranch      string             `json:"base_branch,omitempty"`
	CheckoutSHA     string             `json:"checkout_sha,omitempty"`
	UnifiedDiff     string             `json:"unified_diff,omitempty"`
	SandboxMetadata map[string]string  `json:"sandbox_metadata,omitempty"`
}

// SandboxRunResult reports the outcome of a command executed in a sandbox.
type SandboxRunResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// RemoteCommands defines the high-level inspection commands used by review agents and tools inside a sandbox.
type RemoteCommands interface {
	Grep(ctx context.Context, pattern, path, glob string) (string, error)
	Read(ctx context.Context, path string, start, end int) (string, error)
	ListDir(ctx context.Context, path string, maxDepth int) (string, error)
	Exec(ctx context.Context, command string) (*SandboxRunResult, error)
}

// SandboxInstance represents an active, isolated execution environment with a cloned repository.
type SandboxInstance interface {
	GetID() uuid.UUID
	GetTier() SandboxTier
	GetRepoDir() string
	GetBaseBranch() string
	RemoteCommands() RemoteCommands
	Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*SandboxRunResult, error)
	ReadFile(relPath string) ([]byte, error)
	WriteFile(relPath string, data []byte) error
	Cleanup(ctx context.Context) error

	// Backward-compatibility bridge for ISandbox
	RunCommand(ctx context.Context, req CommandRequest) (*CommandResult, error)
	Destroy(ctx context.Context) error
}

// ISandboxProvider defines the factory contract for creating isolated code review sandboxes.
type ISandboxProvider interface {
	IsAvailable() bool
	CreateSandboxWithRepo(ctx context.Context, params CreateSandboxParams) (SandboxInstance, error)
}

// CommandRequest specifies an executable command to run inside a sandbox (compatibility).
type CommandRequest struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"work_dir"`
	Env        map[string]string `json:"env"`
	TimeoutSec int               `json:"timeout_sec"`
}

// CommandResult reports the outcome of a command executed in a sandbox (compatibility).
type CommandResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// ISandbox defines unified sandbox capabilities across local worktree and microVM (compatibility).
type ISandbox interface {
	GetID() uuid.UUID
	GetTier() SandboxTier
	WriteFile(relPath string, data []byte) error
	ReadFile(relPath string) ([]byte, error)
	RunCommand(ctx context.Context, req CommandRequest) (*CommandResult, error)
	Destroy(ctx context.Context) error
}

// ResolveRepoPath validates that path is relative to repoDir and does not contain ".." traversal.
func ResolveRepoPath(repoDir, relPath string) (string, error) {
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") {
		return "", errors.New("security violation: absolute paths are not allowed in sandbox")
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", errors.New("security violation: path traversal using '..' is not allowed in sandbox")
	}
	return filepath.Join(repoDir, cleaned), nil
}

// ShSingleQuote safely wraps a string in single quotes for shell command invocation.
func ShSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
