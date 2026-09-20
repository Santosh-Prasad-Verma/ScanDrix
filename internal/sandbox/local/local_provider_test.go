package local_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/local"
	"github.com/scandrix/backend/pkg/models"
)

// setupTestRepo creates a local git repository to serve as the clone source.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\noutput: %s", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.email", "test@test.local")
	run("config", "user.name", "Test Author")

	// Create sample files
	mainGo := []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello Scandrix\")\n}\n")
	readme := []byte("# Scandrix Sandbox Test\nTesting local git sandbox containment.\n")

	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), mainGo, 0640); err != nil {
		t.Fatalf("failed writing main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), readme, 0640); err != nil {
		t.Fatalf("failed writing README.md: %v", err)
	}

	run("add", ".")
	run("commit", "-m", "Initial commit")
	run("branch", "-M", "main")

	return repoDir
}

func TestLocalSandboxLifecycleAndRemoteCommands(t *testing.T) {
	sourceRepo := setupTestRepo(t)
	ctx := context.Background()

	provider := local.NewLocalSandboxProvider()
	if !provider.IsAvailable() {
		t.Fatal("expected LocalSandboxProvider to be available")
	}

	inst, err := provider.CreateSandboxWithRepo(ctx, contracts.CreateSandboxParams{
		CloneURL:   sourceRepo,
		Branch:     "main",
		Platform:   models.ProviderGitHub,
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateSandboxWithRepo failed: %v", err)
	}
	defer func() { _ = inst.Cleanup(ctx) }()

	if inst.GetTier() != contracts.TierWorktree {
		t.Errorf("expected TierWorktree, got %s", inst.GetTier())
	}

	// 1. Read existing repository file
	mainContent, err := inst.ReadFile("main.go")
	if err != nil {
		t.Fatalf("failed reading main.go: %v", err)
	}
	if !strings.Contains(string(mainContent), "Hello Scandrix") {
		t.Errorf("expected content to contain Hello Scandrix, got: %s", string(mainContent))
	}

	// 2. Write staged file
	err = inst.WriteFile("staged/test.txt", []byte("staged test content"))
	if err != nil {
		t.Fatalf("failed writing staged file: %v", err)
	}
	stagedRead, err := inst.ReadFile("staged/test.txt")
	if err != nil || string(stagedRead) != "staged test content" {
		t.Fatalf("staged content mismatch: %s", string(stagedRead))
	}

	// 3. RemoteCommands: Grep
	rc := inst.RemoteCommands()
	grepRes, err := rc.Grep(ctx, "Hello Scandrix", "main.go", "")
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	if !strings.Contains(grepRes, "Hello Scandrix") {
		t.Errorf("grep expected to find pattern, got: %s", grepRes)
	}

	// 4. RemoteCommands: Read with line bounds
	lines, err := rc.Read(ctx, "main.go", 1, 3)
	if err != nil {
		t.Fatalf("bounded read failed: %v", err)
	}
	if !strings.Contains(lines, "package main") {
		t.Errorf("expected first line to be package main, got: %s", lines)
	}

	// 5. RemoteCommands: ListDir
	listing, err := rc.ListDir(ctx, ".", 2)
	if err != nil {
		t.Fatalf("listDir failed: %v", err)
	}
	if !strings.Contains(listing, "main.go") || !strings.Contains(listing, "README.md") {
		t.Errorf("expected listing to contain main.go and README.md, got: %s", listing)
	}

	// 6. RemoteCommands: Exec
	execRes, err := rc.Exec(ctx, "ls -1")
	if err != nil || execRes.ExitCode != 0 {
		t.Fatalf("exec failed: %v (exit %d)", err, execRes.ExitCode)
	}
	if !strings.Contains(execRes.Stdout, "main.go") {
		t.Errorf("expected ls stdout to contain main.go, got: %s", execRes.Stdout)
	}

	// 7. Cleanup
	repoDir := inst.GetRepoDir()
	err = inst.Cleanup(ctx)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Errorf("expected temporary directory %s to be deleted after cleanup", repoDir)
	}
}

func TestLocalSandboxPathTraversalRejection(t *testing.T) {
	sourceRepo := setupTestRepo(t)
	ctx := context.Background()

	provider := local.NewLocalSandboxProvider()
	inst, err := provider.CreateSandboxWithRepo(ctx, contracts.CreateSandboxParams{
		CloneURL: sourceRepo,
		Branch:   "main",
		Platform: models.ProviderGitHub,
	})
	if err != nil {
		t.Fatalf("CreateSandboxWithRepo failed: %v", err)
	}
	defer func() { _ = inst.Cleanup(ctx) }()

	maliciousPaths := []string{
		"../../evil.sh",
		"../../../etc/passwd",
		"/etc/passwd",
		"sub/../../escape.txt",
	}

	for _, p := range maliciousPaths {
		err := inst.WriteFile(p, []byte("echo pwned"))
		if err == nil {
			t.Errorf("expected WriteFile to reject %q, but succeeded", p)
		}

		_, err = inst.ReadFile(p)
		if err == nil {
			t.Errorf("expected ReadFile to reject %q, but succeeded", p)
		}
	}
}

func TestLocalSandboxGitHooksDisabled(t *testing.T) {
	// Setup an upstream repo with a malicious hook
	sourceRepo := setupTestRepo(t)
	ctx := context.Background()

	sentinelFile := filepath.Join(t.TempDir(), "hook_ran.txt")
	hookScript := []byte("#!/bin/sh\necho 'MALICIOUS_HOOK_EXECUTED' > " + sentinelFile + "\n")
	hookPath := filepath.Join(sourceRepo, ".git", "hooks", "post-checkout")
	_ = os.WriteFile(hookPath, hookScript, 0755)

	provider := local.NewLocalSandboxProvider()
	inst, err := provider.CreateSandboxWithRepo(ctx, contracts.CreateSandboxParams{
		CloneURL: sourceRepo,
		Branch:   "main",
		Platform: models.ProviderGitHub,
	})
	if err != nil {
		t.Fatalf("CreateSandboxWithRepo failed: %v", err)
	}
	defer func() { _ = inst.Cleanup(ctx) }()

	// Verify that the hook was blocked by core.hooksPath=/dev/null
	if _, err := os.Stat(sentinelFile); !os.IsNotExist(err) {
		t.Fatalf("SECURITY VIOLATION: untrusted git hook executed during sandbox checkout!")
	}
}
