package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/hooks"
)

func TestGitHooksLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-git-hook-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create fake .git/hooks directory
	gitHooksDir := filepath.Join(tempDir, ".git", "hooks")
	if err := os.MkdirAll(gitHooksDir, 0755); err != nil {
		t.Fatalf("failed creating fake git dir: %v", err)
	}

	// 1. Check initial status (no hooks)
	status, err := hooks.Status(tempDir)
	if err != nil || !status.GitRepoDetected || status.PreCommitActive || status.PrePushActive {
		t.Fatalf("unexpected initial hook status: %+v", status)
	}

	// 2. Install pre-commit and pre-push hooks
	if err := hooks.Install(tempDir, true, true, "CRITICAL"); err != nil {
		t.Fatalf("hook install failed: %v", err)
	}

	// 3. Verify installed status
	status, err = hooks.Status(tempDir)
	if err != nil || !status.PreCommitActive || !status.PrePushActive {
		t.Fatalf("expected both hooks to be active, got: %+v", status)
	}

	// 4. Uninstall hooks
	if err := hooks.Uninstall(tempDir); err != nil {
		t.Fatalf("hook uninstall failed: %v", err)
	}

	// 5. Verify uninstalled status
	status, err = hooks.Status(tempDir)
	if err != nil || status.PreCommitActive || status.PrePushActive {
		t.Fatalf("expected hooks to be removed, got: %+v", status)
	}
}

func TestNonDestructiveHookMerge(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-hook-merge-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gitHooksDir := filepath.Join(tempDir, ".git", "hooks")
	_ = os.MkdirAll(gitHooksDir, 0755)

	customPreCommit := `#!/usr/bin/env bash
# Existing Husky/Linter script
echo "Running custom linter..."
npm run lint
`
	preCommitPath := filepath.Join(gitHooksDir, "pre-commit")
	if err := os.WriteFile(preCommitPath, []byte(customPreCommit), 0755); err != nil {
		t.Fatalf("failed writing custom hook: %v", err)
	}

	// Install ScanDrix hook
	if err := hooks.Install(tempDir, true, false, "HIGH"); err != nil {
		t.Fatalf("failed installing hook: %v", err)
	}

	merged, err := os.ReadFile(preCommitPath)
	if err != nil {
		t.Fatalf("failed reading merged hook: %v", err)
	}

	mergedStr := string(merged)
	if !strings.Contains(mergedStr, "Running custom linter...") {
		t.Errorf("custom linter script was clobbered: %s", mergedStr)
	}
	if !strings.Contains(mergedStr, "# scandrix-hook-start") {
		t.Errorf("scandrix hook marker was not inserted: %s", mergedStr)
	}

	// Uninstall ScanDrix hook and verify custom script remains
	if err := hooks.Uninstall(tempDir); err != nil {
		t.Fatalf("failed uninstalling hook: %v", err)
	}

	afterUninstall, err := os.ReadFile(preCommitPath)
	if err != nil {
		t.Fatalf("failed reading hook after uninstall: %v", err)
	}

	afterStr := string(afterUninstall)
	if !strings.Contains(afterStr, "Running custom linter...") {
		t.Errorf("custom script was removed on uninstall: %s", afterStr)
	}
	if strings.Contains(afterStr, "scandrix-hook-start") {
		t.Errorf("scandrix marker was not removed on uninstall: %s", afterStr)
	}
}
