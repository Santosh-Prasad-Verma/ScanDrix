package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	PrefixScandrixSandbox = "scandrix-sandbox-"
	PrefixDrixySandbox    = "drixy-sandbox-"
)

// IsLocalSandboxPath checks if a sandboxId refers to an authentic local sandbox directory.
// SECURITY: Returns true only for direct children of os.TempDir() whose basename starts
// with 'scandrix-sandbox-' or 'drixy-sandbox-', preventing CWE-22 directory traversal.
func IsLocalSandboxPath(sandboxID string) bool {
	if strings.TrimSpace(sandboxID) == "" {
		return false
	}

	tmpDir := filepath.Clean(os.TempDir())
	cleanInput := filepath.Clean(sandboxID)
	basename := filepath.Base(cleanInput)
	resolved := filepath.Join(tmpDir, basename)

	// Must be a direct child of tmpDir
	if filepath.Dir(resolved) != tmpDir {
		return false
	}

	// Must match original path exactly (no traversal escapes)
	if resolved != cleanInput {
		return false
	}

	// Must have authorized prefix
	return strings.HasPrefix(basename, PrefixScandrixSandbox) || strings.HasPrefix(basename, PrefixDrixySandbox)
}

// DeleteLocalSandbox safely deletes a local sandbox directory after verifying path containment.
// Missing directories are treated as successful deletion (idempotent).
func DeleteLocalSandbox(sandboxID string) error {
	if !IsLocalSandboxPath(sandboxID) {
		return fmt.Errorf("invalid local sandbox path: %q", sandboxID)
	}

	if _, err := os.Stat(sandboxID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	return os.RemoveAll(sandboxID)
}
