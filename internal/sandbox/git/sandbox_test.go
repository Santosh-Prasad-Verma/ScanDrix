package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/sandbox/git"
)

func TestEphemeralSandboxManager(t *testing.T) {
	ctx := context.Background()

	tmpBase, err := os.MkdirTemp("", "scandrix_sandbox_test_*")
	if err != nil {
		t.Fatalf("failed creating temp base: %v", err)
	}
	defer os.RemoveAll(tmpBase)

	cfg := sandbox.SandboxConfig{
		BaseDir:       tmpBase,
		MaxFileSizeMB: 2,
		DefaultTTL:    100 * time.Millisecond,
		AutoCleanup:   true,
	}

	mgr, err := sandbox.NewSandboxManager(cfg)
	if err != nil {
		t.Fatalf("failed initializing sandbox manager: %v", err)
	}

	wsID := uuid.New()
	inst, err := mgr.CreateSandbox(ctx, wsID, "acme/auth-service", "sha_commit_abc")
	if err != nil {
		t.Fatalf("failed creating sandbox: %v", err)
	}

	if _, err := os.Stat(inst.WorktreePath); os.IsNotExist(err) {
		t.Fatalf("sandbox worktree directory does not exist on disk: %s", inst.WorktreePath)
	}

	// 1. Safe Write & Read
	testContent := []byte("package main\n\nfunc main() {}\n")
	err = mgr.SafeWriteFile(inst, "cmd/server/main.go", testContent)
	if err != nil {
		t.Fatalf("safe write failed: %v", err)
	}

	readBack, err := mgr.SafeReadFile(inst, "cmd/server/main.go")
	if err != nil {
		t.Fatalf("safe read failed: %v", err)
	}
	if string(readBack) != string(testContent) {
		t.Fatalf("read content mismatch: got %s, want %s", string(readBack), string(testContent))
	}

	// 2. Directory Traversal Attack Defense
	maliciousPaths := []string{
		"../../etc/passwd",
		"../../../var/log/syslog",
		"pkg/../../../../../../etc/shadow",
		"/etc/passwd",
		"/tmp/evil.sh",
	}

	for _, badPath := range maliciousPaths {
		err := mgr.SafeWriteFile(inst, badPath, []byte("evil"))
		if err == nil {
			t.Fatalf("security violation: expected error for malicious path '%s', but write succeeded", badPath)
		}
		if !strings.Contains(err.Error(), "containment violation") && !strings.Contains(err.Error(), "prohibited") {
			t.Fatalf("unexpected error message for '%s': %v", badPath, err)
		}

		_, err = mgr.SafeReadFile(inst, badPath)
		if err == nil {
			t.Fatalf("security violation: expected error for malicious read '%s', but read succeeded", badPath)
		}
	}

	// 3. Normal Cleanup
	err = mgr.Cleanup(inst)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if !inst.CleanedUp {
		t.Fatal("expected instance marked cleaned up")
	}
	if _, err := os.Stat(inst.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected sandbox dir removed after cleanup: %s", inst.WorktreePath)
	}

	// 4. Stale Sandbox Reclamation
	inst2, err := mgr.CreateSandbox(ctx, wsID, "acme/stale-service", "sha_stale_123")
	if err != nil {
		t.Fatalf("failed creating second sandbox: %v", err)
	}

	// Fast forward time past TTL
	reclaimed := mgr.ReclaimStaleSandboxes(ctx, time.Now().UTC().Add(1*time.Second))
	if reclaimed != 1 {
		t.Fatalf("expected 1 reclaimed sandbox, got %d", reclaimed)
	}
	if _, err := os.Stat(inst2.WorktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale sandbox directory to be deleted: %s", inst2.WorktreePath)
	}
}

func TestValidatePathContainmentDirectly(t *testing.T) {
	root := filepath.Clean("/tmp/sandbox_root_123")

	// Valid subpath
	p, err := sandbox.ValidatePathContainment(root, "src/components/button.tsx")
	if err != nil || !strings.HasPrefix(p, root) {
		t.Fatalf("expected valid path containment, got %s (err: %v)", p, err)
	}

	// Invalid traversal
	_, err = sandbox.ValidatePathContainment(root, "../outside.txt")
	if err == nil {
		t.Fatal("expected error for ../outside.txt traversal")
	}
}
