package null

import (
	"context"
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

func (s *NullSandboxInstance) Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*sandbox.SandboxRunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.destroyed {
		return nil, fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	return &sandbox.SandboxRunResult{
		Stdout:   "null run: " + command,
		ExitCode: 0,
		Duration: time.Millisecond,
	}, nil
}

func (s *NullSandboxInstance) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed = true
	s.files = nil
	return nil
}

// RemoteCommands implementation
func (s *NullSandboxInstance) Grep(ctx context.Context, pattern, path, glob string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.destroyed {
		return "", fmt.Errorf("null sandbox %s destroyed", s.id)
	}
	_, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}
	return "No matches found.", nil
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
