package null

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sandbox "github.com/scandrix/backend/internal/sandbox/contracts"
)

// NullSandboxProvider implements sandbox.ISandboxProvider as an in-memory test double.
type NullSandboxProvider struct{}

func NewNullSandboxProvider() *NullSandboxProvider {
	return &NullSandboxProvider{}
}

func (p *NullSandboxProvider) IsAvailable() bool {
	return true
}

func (p *NullSandboxProvider) CreateSandboxWithRepo(ctx context.Context, params sandbox.CreateSandboxParams) (sandbox.SandboxInstance, error) {
	inst := &NullSandboxInstance{
		id:         uuid.New(),
		baseBranch: params.BaseBranch,
		repoDir:    "/null/repo",
		files:      make(map[string][]byte),
	}

	// If unifiedDiff is provided, stage it in a dummy patch file
	if params.UnifiedDiff != "" {
		inst.files["cli.patch"] = []byte(params.UnifiedDiff)
	}

	return inst, nil
}

// NullSandboxInstance represents an in-memory sandbox.
type NullSandboxInstance struct {
	mu         sync.RWMutex
	id         uuid.UUID
	baseBranch string
	repoDir    string
	files      map[string][]byte
	destroyed  bool
}

// NewNullSandboxInstance constructs an initialized NullSandboxInstance.
func NewNullSandboxInstance() *NullSandboxInstance {
	return &NullSandboxInstance{
		id:      uuid.New(),
		repoDir: "/null/repo",
		files:   make(map[string][]byte),
	}
}

func (s *NullSandboxInstance) GetID() uuid.UUID {
	return s.id
}

func (s *NullSandboxInstance) GetTier() sandbox.SandboxTier {
	return sandbox.TierNull
}

func (s *NullSandboxInstance) GetRepoDir() string {
	return s.repoDir
}

func (s *NullSandboxInstance) GetBaseBranch() string {
	return s.baseBranch
}

func (s *NullSandboxInstance) RemoteCommands() sandbox.RemoteCommands {
	return s
}

func (s *NullSandboxInstance) WriteFile(relPath string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	_, err := sandbox.ResolveRepoPath(s.repoDir, relPath)
	if err != nil {
		return err
	}
	s.files[relPath] = data
	return nil
}

func (s *NullSandboxInstance) ReadFile(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.destroyed {
		return nil, fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	_, err := sandbox.ResolveRepoPath(s.repoDir, relPath)
	if err != nil {
		return nil, err
	}
	data, ok := s.files[relPath]
	if !ok {
		return nil, fmt.Errorf("file %s not found", relPath)
	}
	return data, nil
}

// Run does not execute the command. It reports a non-zero exit status and an
// explicit error rather than ExitCode 0, which would present a command that
// never ran as a clean pass (AUDIT_REMEDIATION.md F-38).
func (s *NullSandboxInstance) Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*sandbox.SandboxRunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.destroyed {
		return nil, fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	return nil, fmt.Errorf("null sandbox: refusing to report a result for %q; no command was executed "+
		"(set a real sandbox provider such as E2B, or ALLOW_UNSANDBOXED_COMMAND_EXECUTION=true in development)", command)
}

func (s *NullSandboxInstance) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed = true
	s.files = nil
	return nil
}

// RemoteCommands implementation
// ErrNoSearchBackend is returned instead of a fabricated "no matches" result.
//
// The null provider previously returned "No matches found." without searching
// anything, so a vulnerability-pattern scan over it reported the code as clean
// (AUDIT_REMEDIATION.md F-39). Per AGENTS.md Rule 2.7.2 an absent result must
// be reported as absent, never substituted with a plausible empty answer.
var ErrNoSearchBackend = errors.New(
	"null sandbox: no search backend is configured; a pattern search cannot be performed and must not be reported as clean",
)

func (s *NullSandboxInstance) Grep(ctx context.Context, pattern, path, glob string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.destroyed {
		return "", fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	if _, err := sandbox.ResolveRepoPath(s.repoDir, path); err != nil {
		return "", err
	}
	// Honest failure: the search did not run, so it has no result.
	return "", ErrNoSearchBackend
}

func (s *NullSandboxInstance) Read(ctx context.Context, path string, start, end int) (string, error) {
	data, err := s.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if start <= 0 && end <= 0 {
		return string(data), nil
	}
	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return "", nil
	}
	if end > len(lines) || end <= 0 {
		end = len(lines)
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}

func (s *NullSandboxInstance) ListDir(ctx context.Context, path string, maxDepth int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}
	var listing []string
	for k := range s.files {
		listing = append(listing, s.repoDir+"/"+k)
	}
	return strings.Join(listing, "\n"), nil
}

func (s *NullSandboxInstance) Exec(ctx context.Context, command string) (*sandbox.SandboxRunResult, error) {
	return s.Run(ctx, command, nil, 10*time.Second)
}

// Compatibility methods
func (s *NullSandboxInstance) RunCommand(ctx context.Context, req sandbox.CommandRequest) (*sandbox.CommandResult, error) {
	res, err := s.Run(ctx, req.Command, req.Env, time.Duration(req.TimeoutSec)*time.Second)
	if err != nil {
		return nil, err
	}
	return &sandbox.CommandResult{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
		Duration: res.Duration,
	}, nil
}

func (s *NullSandboxInstance) Destroy(ctx context.Context) error {
	return s.Cleanup(ctx)
}
