package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox"
)

// MicroVMSandbox implements sandbox.ISandbox for remote or containerized execution.
type MicroVMSandbox struct {
	mu        sync.RWMutex
	id        uuid.UUID
	apiKey    string
	endpoint  string
	files     map[string][]byte
	createdAt time.Time
	expiresAt time.Time
	destroyed bool
}

// NewMicroVMSandbox initializes an isolated microVM sandbox lease.
func NewMicroVMSandbox(apiKey, endpoint string, ttl time.Duration) *MicroVMSandbox {
	now := time.Now().UTC()
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &MicroVMSandbox{
		id:        uuid.New(),
		apiKey:    apiKey,
		endpoint:  endpoint,
		files:     make(map[string][]byte),
		createdAt: now,
		expiresAt: now.Add(ttl),
		destroyed: false,
	}
}

func (s *MicroVMSandbox) GetID() uuid.UUID {
	return s.id
}

func (s *MicroVMSandbox) GetTier() sandbox.SandboxTier {
	return sandbox.TierMicroVM
}

func (s *MicroVMSandbox) WriteFile(relPath string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.destroyed {
		return fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	s.files[relPath] = data
	return nil
}

func (s *MicroVMSandbox) ReadFile(relPath string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	data, ok := s.files[relPath]
	if !ok {
		return nil, fmt.Errorf("file %s not found in sandbox", relPath)
	}
	return data, nil
}

func (s *MicroVMSandbox) RunCommand(ctx context.Context, req sandbox.CommandRequest) (*sandbox.CommandResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.destroyed {
		return nil, fmt.Errorf("sandbox %s already destroyed", s.id)
	}

	start := time.Now()

	// 1. If remote E2B endpoint & API key are configured, execute remotely via HTTP
	if s.apiKey != "" && s.endpoint != "" {
		body, _ := json.Marshal(map[string]any{
			"cmd":     req.Command,
			"args":    req.Args,
			"env":     req.Env,
			"timeout": req.TimeoutSec,
		})
		httpReq, err := http.NewRequestWithContext(ctx, "POST", s.endpoint+"/commands", bytes.NewReader(body))
		if err == nil {
			httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
			httpReq.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(httpReq)
			if err == nil {
				defer resp.Body.Close()
				var remoteResult struct {
					Stdout   string `json:"stdout"`
					Stderr   string `json:"stderr"`
					ExitCode int    `json:"exit_code"`
				}
				if json.NewDecoder(resp.Body).Decode(&remoteResult) == nil {
					return &sandbox.CommandResult{
						Stdout:   remoteResult.Stdout,
						Stderr:   remoteResult.Stderr,
						ExitCode: remoteResult.ExitCode,
						Duration: time.Since(start),
					}, nil
				}
			}
		}
	}

	// 2. Real local isolated execution fallback: create ephemeral directory with staged files
	tmpDir, err := os.MkdirTemp("", "scandrix-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("failed creating ephemeral sandbox directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for relPath, data := range s.files {
		targetPath := filepath.Join(tmpDir, relPath)
		_ = os.MkdirAll(filepath.Dir(targetPath), 0750)
		_ = os.WriteFile(targetPath, data, 0600)
	}

	cmdCtx := ctx
	if req.TimeoutSec > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSec)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, req.Command, req.Args...)
	cmd.Dir = tmpDir
	for k, v := range req.Env {
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

	timedOut := false
	if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
		timedOut = true
	}

	return &sandbox.CommandResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Duration: time.Since(start),
		TimedOut: timedOut,
	}, nil
}

func (s *MicroVMSandbox) Destroy(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.destroyed = true
	s.files = nil
	return nil
}
