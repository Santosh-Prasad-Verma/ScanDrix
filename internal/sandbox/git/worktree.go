package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SandboxManager orchestrates ephemeral filesystem sandboxes for code analysis.
type SandboxManager struct {
	mu        sync.RWMutex
	cfg       SandboxConfig
	sandboxes map[uuid.UUID]*WorktreeInstance
}

// NewSandboxManager initializes the sandbox manager.
func NewSandboxManager(cfg SandboxConfig) (*SandboxManager, error) {
	if cfg.BaseDir == "" {
		cfg.BaseDir = filepath.Join(os.TempDir(), "scandrix_sandboxes")
	}
	if cfg.MaxFileSizeMB <= 0 {
		cfg.MaxFileSizeMB = 20
	}
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 15 * time.Minute
	}

	if err := os.MkdirAll(cfg.BaseDir, 0750); err != nil {
		return nil, fmt.Errorf("failed creating sandbox base dir: %w", err)
	}

	return &SandboxManager{
		cfg:       cfg,
		sandboxes: make(map[uuid.UUID]*WorktreeInstance),
	}, nil
}

// CreateSandbox provisions an isolated directory for a specific commit checkout.
func (m *SandboxManager) CreateSandbox(ctx context.Context, wsID uuid.UUID, repoNamespace, commitSHA string) (*WorktreeInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New()
	worktreePath := filepath.Join(m.cfg.BaseDir, id.String())

	if err := os.MkdirAll(worktreePath, 0750); err != nil {
		return nil, fmt.Errorf("failed provisioning sandbox directory: %w", err)
	}

	now := time.Now().UTC()
	inst := &WorktreeInstance{
		ID:            id,
		WorkspaceID:   wsID,
		RepoNamespace: repoNamespace,
		CommitSHA:     commitSHA,
		WorktreePath:  worktreePath,
		CreatedAt:     now,
		ExpiresAt:     now.Add(m.cfg.DefaultTTL),
		CleanedUp:     false,
	}

	m.sandboxes[id] = inst
	return inst, nil
}

// SafeWriteFile writes file contents into the sandbox, asserting path containment and size limits.
func (m *SandboxManager) SafeWriteFile(inst *WorktreeInstance, relPath string, data []byte) error {
	if int64(len(data)) > m.cfg.MaxFileSizeMB*1024*1024 {
		return fmt.Errorf("file size %d bytes exceeds maximum limit of %d MB", len(data), m.cfg.MaxFileSizeMB)
	}

	safePath, err := ValidatePathContainment(inst.WorktreePath, relPath)
	if err != nil {
		return fmt.Errorf("path containment violation: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(safePath), 0750); err != nil {
		return fmt.Errorf("failed creating parent directory: %w", err)
	}

	return os.WriteFile(safePath, data, 0640)
}

// SafeReadFile reads file contents from the sandbox with security bounds checking.
func (m *SandboxManager) SafeReadFile(inst *WorktreeInstance, relPath string) ([]byte, error) {
	safePath, err := ValidatePathContainment(inst.WorktreePath, relPath)
	if err != nil {
		return nil, fmt.Errorf("path containment violation: %w", err)
	}

	f, err := os.Open(safePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	limitReader := io.LimitReader(f, m.cfg.MaxFileSizeMB*1024*1024+1)
	data, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	if int64(len(data)) > m.cfg.MaxFileSizeMB*1024*1024 {
		return nil, fmt.Errorf("file exceeds maximum size limit of %d MB", m.cfg.MaxFileSizeMB)
	}

	return data, nil
}

// Cleanup tears down a sandbox and frees disk space.
func (m *SandboxManager) Cleanup(inst *WorktreeInstance) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if inst.CleanedUp {
		return nil
	}

	err := os.RemoveAll(inst.WorktreePath)
	inst.CleanedUp = true
	delete(m.sandboxes, inst.ID)
	return err
}

// ReclaimStaleSandboxes finds and purges expired sandboxes.
func (m *SandboxManager) ReclaimStaleSandboxes(ctx context.Context, now time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	reclaimed := 0
	for id, inst := range m.sandboxes {
		if now.After(inst.ExpiresAt) {
			_ = os.RemoveAll(inst.WorktreePath)
			inst.CleanedUp = true
			delete(m.sandboxes, id)
			reclaimed++
		}
	}
	return reclaimed
}
