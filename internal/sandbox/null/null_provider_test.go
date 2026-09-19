package null_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/null"
	"github.com/scandrix/backend/pkg/models"
)

func TestNullSandboxProvider(t *testing.T) {
	ctx := context.Background()
	p := null.NewNullSandboxProvider()

	if !p.IsAvailable() {
		t.Fatal("expected NullSandboxProvider to be available")
	}

	inst, err := p.CreateSandboxWithRepo(ctx, contracts.CreateSandboxParams{
		CloneURL:    "https://github.com/org/repo.git",
		Branch:      "main",
		Platform:    models.ProviderGitHub,
		UnifiedDiff: "diff --git a/test.txt b/test.txt",
	})
	if err != nil {
		t.Fatalf("failed creating null sandbox: %v", err)
	}
	defer func() { _ = inst.Cleanup(ctx) }()

	if inst.GetTier() != contracts.TierNull {
		t.Errorf("expected TierNull, got %s", inst.GetTier())
	}

	// 1. Write and Read file
	err = inst.WriteFile("hello.txt", []byte("line1\nline2\nline3\n"))
	if err != nil {
		t.Fatalf("failed writing file: %v", err)
	}

	data, err := inst.ReadFile("hello.txt")
	if err != nil || string(data) != "line1\nline2\nline3\n" {
		t.Fatalf("read mismatch: %s", string(data))
	}

	// 2. RemoteCommands
	rc := inst.RemoteCommands()
	readLines, err := rc.Read(ctx, "hello.txt", 1, 2)
	if err != nil {
		t.Fatalf("read lines failed: %v", err)
	}
	if !strings.Contains(readLines, "line1\nline2") {
		t.Errorf("unexpected read lines: %s", readLines)
	}

	grepOut, err := rc.Grep(ctx, "pattern", "hello.txt", "")
	if err != nil || grepOut != "No matches found." {
		t.Errorf("unexpected grep output: %s", grepOut)
	}

	listOut, err := rc.ListDir(ctx, ".", 2)
	if err != nil || !strings.Contains(listOut, "hello.txt") {
		t.Errorf("unexpected listDir output: %s", listOut)
	}

	execRes, err := rc.Exec(ctx, "echo test")
	if err != nil || execRes.ExitCode != 0 {
		t.Errorf("unexpected exec result: %v, exit %d", err, execRes.ExitCode)
	}

	// 3. Path Traversal rejection
	err = inst.WriteFile("../../escape.txt", []byte("evil"))
	if err == nil {
		t.Error("expected traversal path to be rejected")
	}

	_, err = inst.ReadFile("../../escape.txt")
	if err == nil {
		t.Error("expected traversal read to be rejected")
	}

	// 4. Run command
	runRes, err := inst.Run(ctx, "whoami", nil, 5*time.Second)
	if err != nil || runRes.ExitCode != 0 {
		t.Errorf("run failed: %v", err)
	}

	// 5. Cleanup
	err = inst.Cleanup(ctx)
	if err != nil {
		t.Errorf("cleanup failed: %v", err)
	}
	if err := inst.WriteFile("after.txt", []byte("bad")); err == nil {
		t.Error("expected write after cleanup to fail")
	}
}
