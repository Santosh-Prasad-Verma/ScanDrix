package local

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sandbox "github.com/scandrix/backend/internal/sandbox/contracts"
)

const (
	defaultCloneTimeout   = 2 * time.Minute
	defaultCommandTimeout = 30 * time.Second
	maxOutputBuffer       = 5 * 1024 * 1024 // 5 MB ceiling
)

// LocalSandboxProvider implements sandbox.ISandboxProvider using isolated local git checkouts.
type LocalSandboxProvider struct{}

func NewLocalSandboxProvider() *LocalSandboxProvider {
	return &LocalSandboxProvider{}
}

func (p *LocalSandboxProvider) IsAvailable() bool {
	return true
}

func (p *LocalSandboxProvider) CreateSandboxWithRepo(ctx context.Context, params sandbox.CreateSandboxParams) (sandbox.SandboxInstance, error) {
	tempDir, err := os.MkdirTemp("", "scandrix-sandbox-local-*")
	if err != nil {
		return nil, fmt.Errorf("failed creating local sandbox temporary dir: %w", err)
	}

	inst := &LocalSandboxInstance{
		id:         uuid.New(),
		repoDir:    tempDir,
		baseBranch: params.BaseBranch,
	}

	cleanupOnFailure := true
	defer func() {
		if cleanupOnFailure {
			_ = os.RemoveAll(tempDir)
		}
	}()

	// 1. git init
	cmd := exec.CommandContext(ctx, "git", "init", tempDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git init failed: %w (output: %s)", err, string(out))
	}

	// 2. Disable git hooks to prevent arbitrary code execution from untrusted repos (CWE-78 mitigation)
	cmd = exec.CommandContext(ctx, "git", "-C", tempDir, "config", "core.hooksPath", "/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("disabling git hooks failed: %w (output: %s)", err, string(out))
	}

	// 3. Set dummy user name/email for 3-way merge commit fallbacks
	_ = exec.CommandContext(ctx, "git", "-C", tempDir, "config", "user.email", "scandrix-local@scandrix.internal").Run()
	_ = exec.CommandContext(ctx, "git", "-C", tempDir, "config", "user.name", "ScanDrix Local Sandbox").Run()

	// 4. Resolve refspecs and auth header
	authHeader, err := sandbox.BuildAuthHeader(params.Platform, params.AuthToken, params.AuthUsername)
	if err != nil {
		return nil, fmt.Errorf("failed generating git auth header: %w", err)
	}

	refspec := params.CheckoutSHA
	localRef := "cli-base"
	if refspec == "" {
		if params.PRNumber > 0 {
			refspec = sandbox.GetPRRefspec(params.Platform, params.PRNumber, params.CloneURL, params.Branch)
			localRef = "pr-head"
		} else {
			refspec = "refs/heads/" + params.Branch
			localRef = "cli-head"
		}
	}

	// 5. Shallow fetch repository (never embedding token in URL or command line args)
	fetchArgs := []string{"-C", tempDir}
	if authHeader != "" {
		fetchArgs = append(fetchArgs, "-c", "http.extraHeader="+authHeader)
	}
	fetchArgs = append(fetchArgs, "fetch", "--depth=1", params.CloneURL, fmt.Sprintf("%s:%s", refspec, localRef))

	cloneCtx, cancel := context.WithTimeout(ctx, defaultCloneTimeout)
	defer cancel()

	cmd = exec.CommandContext(cloneCtx, "git", fetchArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git fetch failed (exit: %v): %s", err, string(out))
	}

	// 6. Checkout ref
	cmd = exec.CommandContext(ctx, "git", "-C", tempDir, "checkout", localRef)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git checkout %s failed: %w (output: %s)", localRef, err, string(out))
	}

	// 7. Configure dummy remote & block pushes
	_ = exec.CommandContext(ctx, "git", "-C", tempDir, "remote", "add", "origin", params.CloneURL).Run()
	_ = exec.CommandContext(ctx, "git", "-C", tempDir, "remote", "set-url", "--push", "origin", "no-push-allowed").Run()

	// 8. Apply unified diff if provided (CLI unpushed changes or uncommitted work)
	if params.UnifiedDiff != "" {
		patchPath := filepath.Join(tempDir, ".scandrix-cli.patch")
		if err := os.WriteFile(patchPath, []byte(params.UnifiedDiff), 0600); err == nil {
			applyCmd := exec.CommandContext(ctx, "git", "-C", tempDir, "apply", "--3way", "--whitespace=nowarn", patchPath)
			if err := applyCmd.Run(); err != nil {
				// Fallback to more permissive whitespace fixing
				_ = exec.CommandContext(ctx, "git", "-C", tempDir, "apply", "--whitespace=fix", "--reject", patchPath).Run()
			}
			_ = os.Remove(patchPath)
		}
	}

	// 9. Fetch base branch if specified
	if params.BaseBranch != "" {
		baseRefArgs := []string{"-C", tempDir}
		if authHeader != "" {
			baseRefArgs = append(baseRefArgs, "-c", "http.extraHeader="+authHeader)
		}
		baseRefArgs = append(baseRefArgs, "fetch", "--depth=1", params.CloneURL,
			fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", params.BaseBranch, params.BaseBranch))
		_ = exec.CommandContext(ctx, "git", baseRefArgs...).Run()
	}

	cleanupOnFailure = false
	return inst, nil
}

// LocalSandboxInstance represents a local worktree sandbox.
type LocalSandboxInstance struct {
	mu         sync.RWMutex
	id         uuid.UUID
	repoDir    string
	baseBranch string
	destroyed  bool
}

// NewLocalSandboxInstance creates an instance wrapping an existing local sandbox directory.
func NewLocalSandboxInstance(repoDir, baseBranch string) *LocalSandboxInstance {
	return &LocalSandboxInstance{
		id:         uuid.New(),
		repoDir:    repoDir,
		baseBranch: baseBranch,
	}
}


func (s *LocalSandboxInstance) GetID() uuid.UUID {
	return s.id
}

func (s *LocalSandboxInstance) GetTier() sandbox.SandboxTier {
	return sandbox.TierWorktree
}

func (s *LocalSandboxInstance) GetRepoDir() string {
	return s.repoDir
}

func (s *LocalSandboxInstance) GetBaseBranch() string {
	return s.baseBranch
}

func (s *LocalSandboxInstance) RemoteCommands() sandbox.RemoteCommands {
	return s
}

func (s *LocalSandboxInstance) WriteFile(relPath string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.destroyed {
		return fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, relPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return fmt.Errorf("failed creating parent directory: %w", err)
	}

	return os.WriteFile(targetPath, data, 0640)
}

func (s *LocalSandboxInstance) ReadFile(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, relPath)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(targetPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(io.LimitReader(f, maxOutputBuffer))
}

func (s *LocalSandboxInstance) Run(ctx context.Context, command string, envs map[string]string, timeout time.Duration) (*sandbox.SandboxRunResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)
	cmd.Dir = s.repoDir

	for k, v := range envs {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	execErr := cmd.Run()
	exitCode := 0
	if execErr != nil {
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	timedOut := errors.Is(cmdCtx.Err(), context.DeadlineExceeded)

	return &sandbox.SandboxRunResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Duration: time.Since(start),
		TimedOut: timedOut,
	}, nil
}

func (s *LocalSandboxInstance) Cleanup(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.destroyed = true
	if s.repoDir != "" {
		return os.RemoveAll(s.repoDir)
	}
	return nil
}

// RemoteCommands methods
func (s *LocalSandboxInstance) Grep(ctx context.Context, pattern, path, glob string) (string, error) {
	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}

	// Check if ripgrep (rg) is available
	if rgPath, err := exec.LookPath("rg"); err == nil && rgPath != "" {
		args := []string{"--no-heading", "-n"}
		if glob != "" {
			args = append(args, "--glob", glob)
		}
		args = append(args, pattern, targetPath)

		cmd := exec.CommandContext(ctx, "rg", args...)
		cmd.Dir = s.repoDir

		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		_ = cmd.Run()
		if outBuf.Len() == 0 {
			if errBuf.Len() > 0 {
				return "Error: " + errBuf.String(), nil
			}
			return "No matches found.", nil
		}
		return outBuf.String(), nil
	}

	// Fallback pure Go directory scanner
	var matches []string
	_ = filepath.Walk(targetPath, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			if matched, _ := filepath.Match(glob, filepath.Base(p)); !matched {
				return nil
			}
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(s.repoDir, p)
		scanner := bufio.NewScanner(f)
		lineNo := 1
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, pattern) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, lineNo, line))
				if len(matches) >= 100 {
					return fmt.Errorf("limit reached")
				}
			}
			lineNo++
		}
		return nil
	})

	if len(matches) == 0 {
		return "No matches found.", nil
	}
	return strings.Join(matches, "\n"), nil
}

func (s *LocalSandboxInstance) Read(ctx context.Context, path string, start, end int) (string, error) {
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

func (s *LocalSandboxInstance) ListDir(ctx context.Context, path string, maxDepth int) (string, error) {
	targetPath, err := sandbox.ResolveRepoPath(s.repoDir, path)
	if err != nil {
		return "", err
	}

	if maxDepth <= 0 {
		maxDepth = 2
	}

	var listings []string
	baseDepth := len(strings.Split(filepath.Clean(targetPath), string(filepath.Separator)))

	_ = filepath.Walk(targetPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}
		depth := len(strings.Split(filepath.Clean(p), string(filepath.Separator))) - baseDepth
		if depth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(s.repoDir, p)
		if rel != "." {
			if info.IsDir() {
				listings = append(listings, rel+"/")
			} else {
				listings = append(listings, rel)
			}
		}
		return nil
	})

	return strings.Join(listings, "\n"), nil
}

func (s *LocalSandboxInstance) Exec(ctx context.Context, command string) (*sandbox.SandboxRunResult, error) {
	return s.Run(ctx, command, nil, defaultCommandTimeout)
}

// Backward-compatibility bridge
func (s *LocalSandboxInstance) RunCommand(ctx context.Context, req sandbox.CommandRequest) (*sandbox.CommandResult, error) {
	cmdStr := req.Command
	if len(req.Args) > 0 {
		cmdStr += " " + strings.Join(req.Args, " ")
	}
	res, err := s.Run(ctx, cmdStr, req.Env, time.Duration(req.TimeoutSec)*time.Second)
	if err != nil {
		return nil, err
	}
	return &sandbox.CommandResult{
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
		ExitCode: res.ExitCode,
		Duration: res.Duration,
		TimedOut: res.TimedOut,
	}, nil
}

func (s *LocalSandboxInstance) Destroy(ctx context.Context) error {
	return s.Cleanup(ctx)
}
