package local_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scandrix/backend/internal/sandbox/local"
)

func TestIsLocalSandboxPath(t *testing.T) {
	tmpDir := os.TempDir()

	validPaths := []string{
		filepath.Join(tmpDir, "scandrix-sandbox-abc123"),
		filepath.Join(tmpDir, "drixy-sandbox-xyz789"),
	}

	for _, p := range validPaths {
		if !local.IsLocalSandboxPath(p) {
			t.Errorf("expected %q to be recognized as valid local sandbox path", p)
		}
	}

	invalidPaths := []string{
		"",
		"/etc/passwd",
		filepath.Join(tmpDir, "other-prefix-123"),
		filepath.Join(tmpDir, "../evil"),
		filepath.Join(tmpDir, "scandrix-sandbox-test", "nested"),
		tmpDir,
		"scandrix-sandbox-relative",
	}

	for _, p := range invalidPaths {
		if local.IsLocalSandboxPath(p) {
			t.Errorf("expected %q to be rejected as local sandbox path", p)
		}
	}
}

func TestDeleteLocalSandbox(t *testing.T) {
	tmpDir := os.TempDir()
	validDir := filepath.Join(tmpDir, "scandrix-sandbox-test-delete")
	if err := os.MkdirAll(validDir, 0700); err != nil {
		t.Fatalf("failed to create temp test dir: %v", err)
	}

	// Successful deletion
	if err := local.DeleteLocalSandbox(validDir); err != nil {
		t.Fatalf("expected successful deletion of %q, got: %v", validDir, err)
	}

	// Idempotent (missing dir returns nil)
	if err := local.DeleteLocalSandbox(validDir); err != nil {
		t.Fatalf("expected nil for already deleted dir, got: %v", err)
	}

	// Invalid path should error
	if err := local.DeleteLocalSandbox("/etc/shadow"); err == nil {
		t.Fatal("expected error when trying to delete invalid path /etc/shadow, got nil")
	}
}
