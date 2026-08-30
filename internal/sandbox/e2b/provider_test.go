package e2b_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/sandbox"
	"github.com/scandrix/backend/internal/sandbox/e2b"
)

func TestMicroVMSandboxRealExecution(t *testing.T) {
	ctx := context.Background()
	sb := e2b.NewMicroVMSandbox("", "", 10*time.Minute)

	// Stage a file in sandbox
	err := sb.WriteFile("hello.txt", []byte("Hello from Scandrix Sandbox"))
	if err != nil {
		t.Fatalf("failed writing file to sandbox: %v", err)
	}

	readBack, err := sb.ReadFile("hello.txt")
	if err != nil || string(readBack) != "Hello from Scandrix Sandbox" {
		t.Fatalf("read back content mismatch: %s", string(readBack))
	}

	// Real command execution in sandbox: read the staged file
	res, err := sb.RunCommand(ctx, sandbox.CommandRequest{
		Command:    "cat",
		Args:       []string{"hello.txt"},
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("run command failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d. stderr: %s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "Hello from Scandrix Sandbox") {
		t.Fatalf("expected stdout to contain staged content, got: %s", res.Stdout)
	}

	// Clean up
	_ = sb.Destroy(ctx)
}
