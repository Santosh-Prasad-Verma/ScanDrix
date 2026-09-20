package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitStatus(t *testing.T) {
	tests := []struct {
		char     string
		expected string
	}{
		{"A", "added"},
		{"?", "added"},
		{"D", "deleted"},
		{"R", "renamed"},
		{"C", "copied"},
		{"U", "unmerged"},
		{"M", "modified"},
		{"", "modified"},
	}

	for _, tc := range tests {
		got := ParseGitStatus(tc.char)
		if got != tc.expected {
			t.Errorf("ParseGitStatus(%q) = %q, want %q", tc.char, got, tc.expected)
		}
	}
}

func TestParseGitNameStatusOutput(t *testing.T) {
	raw := "M\tmain.go\nA\tpkg/util.go\nD\told_file.go\nR100\told_name.go\tnew_name.go\n"
	entries := ParseGitNameStatusOutput(raw)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].File != "main.go" || entries[0].Status != "modified" {
		t.Errorf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].File != "pkg/util.go" || entries[1].Status != "added" {
		t.Errorf("unexpected entry 1: %+v", entries[1])
	}
	if entries[2].File != "old_file.go" || entries[2].Status != "deleted" {
		t.Errorf("unexpected entry 2: %+v", entries[2])
	}
	if entries[3].File != "new_name.go" || entries[3].OldFile != "old_name.go" || entries[3].Status != "renamed" {
		t.Errorf("unexpected entry 3: %+v", entries[3])
	}

	fileList := ListFilesFromNameStatus(raw)
	if len(fileList) != 4 || fileList[3] != "new_name.go" {
		t.Errorf("unexpected file list: %v", fileList)
	}

	statusMap := BuildFileStatusMap(raw)
	if statusMap["main.go"] != "modified" || statusMap["new_name.go"] != "renamed" {
		t.Errorf("unexpected status map: %+v", statusMap)
	}
}

func TestParsePorcelainStatus(t *testing.T) {
	raw := "M  staged_file.go\n M unstaged_file.go\n?? untracked.go\nR  old.go -> new.go\n"
	entries := ParsePorcelainStatus(raw)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if !entries[0].IsStaged || entries[0].File != "staged_file.go" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if !entries[1].IsUnstaged || entries[1].File != "unstaged_file.go" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
	if !entries[2].IsUntracked || entries[2].Status != "added" || entries[2].File != "untracked.go" {
		t.Errorf("entry 2 mismatch: %+v", entries[2])
	}
	if entries[3].Status != "renamed" || entries[3].OldFile != "old.go" || entries[3].File != "new.go" {
		t.Errorf("entry 3 mismatch: %+v", entries[3])
	}
}

func TestIgnorePatterns(t *testing.T) {
	patterns := DefaultExcludedPatterns

	tests := []struct {
		path     string
		expected bool
	}{
		{"package-lock.json", true},
		{"vendor/github.com/foo/bar.go", true},
		{"node_modules/react/index.js", true},
		{"dist/bundle.min.js", true},
		{"src/main.go", false},
		{"internal/cli/git/service.go", false},
		{"assets/logo.png", true},
	}

	for _, tc := range tests {
		got := ShouldIgnoreFile(tc.path, patterns)
		if got != tc.expected {
			t.Errorf("ShouldIgnoreFile(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

func TestHooksLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scandrix-git-hooks-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	res, err := InstallHooks(tmpDir)
	if err != nil {
		t.Fatalf("InstallHooks failed: %v", err)
	}

	if len(res.Installed) != 2 {
		t.Errorf("expected 2 hooks installed, got %d", len(res.Installed))
	}

	// Reinstall idempotency check
	res2, err := InstallHooks(tmpDir)
	if err != nil {
		t.Fatalf("reinstall InstallHooks failed: %v", err)
	}
	if len(res2.AlreadyInstalled) != 2 {
		t.Errorf("expected 2 already installed, got %d", len(res2.AlreadyInstalled))
	}

	// Verify status
	status, err := CheckHooksStatus(tmpDir)
	if err != nil {
		t.Fatalf("CheckHooksStatus failed: %v", err)
	}
	if !status.PrepareCommitMsgInstalled || !status.PrePushInstalled {
		t.Errorf("expected hooks active, got %+v", status)
	}

	// Verify trailer marker presence
	prepareData, err := os.ReadFile(filepath.Join(tmpDir, "prepare-commit-msg"))
	if err != nil {
		t.Fatalf("read prepare-commit-msg failed: %v", err)
	}
	if !strings.Contains(string(prepareData), "ScanDrix-Trace:") {
		t.Errorf("prepare-commit-msg missing ScanDrix-Trace: %s", string(prepareData))
	}

	// Uninstall
	unres, err := UninstallHooks(tmpDir)
	if err != nil {
		t.Fatalf("UninstallHooks failed: %v", err)
	}
	if len(unres.Removed) < 2 {
		t.Errorf("expected at least 2 hooks removed, got %d", len(unres.Removed))
	}

	statusAfter, _ := CheckHooksStatus(tmpDir)
	if statusAfter.PrepareCommitMsgInstalled || statusAfter.PrePushInstalled {
		t.Errorf("hooks should not be installed after uninstall, got %+v", statusAfter)
	}
}

func TestGitServiceCurrentRepo(t *testing.T) {
	svc := NewGitService("")
	ctx := context.Background()

	if !svc.IsGitRepository(ctx) {
		t.Skip("not running inside git repo")
	}

	root, err := svc.GetGitRoot(ctx)
	if err != nil {
		t.Fatalf("GetGitRoot failed: %v", err)
	}
	if root == "" {
		t.Errorf("expected non-empty git root")
	}

	info, err := svc.GetGitInfo(ctx)
	if err != nil {
		t.Fatalf("GetGitInfo failed: %v", err)
	}
	if info.RootPath == "" {
		t.Errorf("expected root path populated in GitInfo")
	}
}
