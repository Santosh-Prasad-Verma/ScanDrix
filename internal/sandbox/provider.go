package sandbox

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SandboxTier identifies the isolation technology tier.
type SandboxTier string

const (
	TierWorktree SandboxTier = "WORKTREE"   // Fast local ephemeral checkout
	TierMicroVM  SandboxTier = "E2B_MICROVM" // Isolated remote container/VM
)

// CommandRequest specifies an executable command to run inside a sandbox.
type CommandRequest struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"work_dir"`
	Env        map[string]string `json:"env"`
	TimeoutSec int               `json:"timeout_sec"`
}

// CommandResult reports the outcome of a command executed in a sandbox.
type CommandResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// ISandbox defines unified sandbox capabilities across local worktree and microVM.
type ISandbox interface {
	GetID() uuid.UUID
	GetTier() SandboxTier
	WriteFile(relPath string, data []byte) error
	ReadFile(relPath string) ([]byte, error)
	RunCommand(ctx context.Context, req CommandRequest) (*CommandResult, error)
	Destroy(ctx context.Context) error
}
