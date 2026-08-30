package e2b_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/sandbox"
	"github.com/scandrix/backend/internal/sandbox/e2b"
)

func TestMicroVMSandboxLifecycle(t *testing.T) {
	ctx := context.Background()
	sb := e2b.NewMicroVMSandbox("test-api-key", "https://api.e2b.dev", 5*time.Minute)

	if sb.GetTier() != sandbox.TierMicroVM {
		t.Errorf("expected tier %s, got %s", sandbox.TierMicroVM, sb.GetTier())
	}

	// 1. Write file
	err := sb.WriteFile("main.go", []byte("package main\nfunc main(){}\n"))
	if err != nil {
		t.Fatalf("failed writing file: %v", err)
	}

	// 2. Read file
	data, err := sb.ReadFile("main.go")
	if err != nil || len(data) == 0 {
		t.Fatalf("failed reading file: %v", err)
	}

	// 3. Run command
	res, err := sb.RunCommand(ctx, sandbox.CommandRequest{
		Command: "go test ./...",
	})
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("command failed: %v", err)
	}

	// 4. Destroy
	_ = sb.Destroy(ctx)
	if err := sb.WriteFile("test.txt", []byte("hello")); err == nil {
		t.Errorf("expected error writing to destroyed sandbox")
	}
}
