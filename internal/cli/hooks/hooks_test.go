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

func TestGitWorktreeHookInstallation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-worktree-test-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Simulate main repo common dir: <tempDir>/main/.git
	mainGit := filepath.Join(tempDir, "main", ".git")
	mainHooks := filepath.Join(mainGit, "hooks")
	_ = os.MkdirAll(mainHooks, 0755)

	// Simulate linked worktree: <tempDir>/worktree-1/.git (file pointing to <mainGit>/worktrees/wt1)
	wtDir := filepath.Join(tempDir, "worktree-1")
	_ = os.MkdirAll(wtDir, 0755)
	wtGitFile := filepath.Join(wtDir, ".git")
	wtTarget := filepath.Join(mainGit, "worktrees", "wt1")
	_ = os.MkdirAll(wtTarget, 0755)
	_ = os.WriteFile(wtGitFile, []byte("gitdir: "+wtTarget+"\n"), 0644)

	// Install hook from the worktree
	if err := hooks.Install(wtDir, true, true, "critical", true); err != nil {
		t.Fatalf("failed installing hook in worktree: %v", err)
	}

	// Verify hook was installed in the common hooks dir
	prePushPath := filepath.Join(mainHooks, "pre-push")
	content, err := os.ReadFile(prePushPath)
	if err != nil {
		t.Fatalf("pre-push hook was not created in common git dir: %v", err)
	}
	if !strings.Contains(string(content), "scandrix-hook-start") {
		t.Errorf("hook content missing scandrix marker: %s", string(content))
	}
	if !strings.Contains(string(content), "--fail-on critical") {
		t.Errorf("hook content missing --fail-on critical: %s", string(content))
	}
}

func TestTraceBlockPreservation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scandrix-trace-preserve-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gitHooksDir := filepath.Join(tempDir, ".git", "hooks")
	_ = os.MkdirAll(gitHooksDir, 0755)

	existingWithTrace := `#!/bin/sh
# scandrix-trace-start
scandrix trace hooks git pre-push
# scandrix-trace-end
`
	prePushPath := filepath.Join(gitHooksDir, "pre-push")
	_ = os.WriteFile(prePushPath, []byte(existingWithTrace), 0755)

	// Install ScanDrix review hook
	if err := hooks.Install(tempDir, false, true, "critical", true); err != nil {
		t.Fatalf("failed installing hook: %v", err)
	}

	content, err := os.ReadFile(prePushPath)
	if err != nil {
		t.Fatalf("failed reading pre-push hook: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "scandrix-trace-start") {
		t.Errorf("trace marker start was stripped during hook install: %s", contentStr)
	}
	if !strings.Contains(contentStr, "scandrix trace hooks git pre-push") {
		t.Errorf("trace command was stripped during hook install: %s", contentStr)
	}
	if !strings.Contains(contentStr, "scandrix-hook-start") {
		t.Errorf("review hook was not added: %s", contentStr)
	}
}

