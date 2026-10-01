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

func TestMicroVMSandboxPathTraversalRejection(t *testing.T) {
	ctx := context.Background()
	sb := e2b.NewMicroVMSandbox("", "", 10*time.Minute)
	defer func() { _ = sb.Destroy(ctx) }()

	maliciousPaths := []string{
		"../../evil.sh",
		"../../../etc/cron.d/malicious",
		"/tmp/pwn.txt",
		"subdir/../../escape.txt",
	}

	for _, p := range maliciousPaths {
		err := sb.WriteFile(p, []byte("echo pwned"))
		if err == nil {
			t.Errorf("expected WriteFile to reject traversal path %q, but got nil", p)
		}
	}
}

func TestMicroVMSandboxProductionFailClosed(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("ALLOW_UNSANDBOXED_COMMAND_EXECUTION", "false")

	ctx := context.Background()
	sb := e2b.NewMicroVMSandbox("", "", 10*time.Minute)
	defer func() { _ = sb.Destroy(ctx) }()

	_, err := sb.RunCommand(ctx, sandbox.CommandRequest{
		Command: "ls",
	})
	if err == nil {
		t.Fatal("expected RunCommand to fail closed in production without E2B key, but succeeded")
	}
	if !strings.Contains(err.Error(), "secure execution failed") {
		t.Fatalf("expected secure execution failure message, got: %v", err)
	}
}

// F-38: the sandbox gate tested only for "production", so a staging deployment
// with no E2B credentials fell through to the development mock, which returned
// exit code 0. A staging run that never reached the sandbox reported a clean
// pass, and a release gate could not tell the difference from a real review.
func TestStagingRequiresIsolation(t *testing.T) {
	p := &e2b.MicroVMSandbox{}
	cmd := "echo hi"

	for _, env := range []string{"staging", "production"} {
		for _, key := range []string{"ENVIRONMENT", "APP_ENV"} {
			t.Run(env+" via "+key, func(t *testing.T) {
				t.Setenv(key, env)
				t.Setenv("ALLOW_UNSANDBOXED_COMMAND_EXECUTION", "")
				t.Setenv("E2B_API_KEY", "")

				res, err := p.Run(context.Background(), cmd, nil, 10*time.Second)
				if err == nil && res != nil && res.ExitCode == 0 {
					t.Fatalf("%s must not succeed via the dev mock (exit 0)", env)
				}
			})
		}
	}
}

// The dev mock itself must never look like success, or a vulnerability scan
// gates on a command that was never run.
func TestDevMockNeverReportsSuccess(t *testing.T) {
	p := &e2b.MicroVMSandbox{}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("APP_ENV", "development")

	res, err := p.Run(context.Background(), "grep -r TODO .", nil, 10*time.Second)
	if err != nil {
		t.Skipf("mock path not reached in this environment: %v", err)
	}
	if res == nil {
		t.Fatal("expected a result")
	}
	if res.ExitCode == 0 {
		t.Error("a command that was never executed must not report exit code 0")
	}
	if !strings.Contains(res.Stdout, "NOT executed") {
		t.Errorf("stdout must state the command was not executed, got %q", res.Stdout)
	}
}
