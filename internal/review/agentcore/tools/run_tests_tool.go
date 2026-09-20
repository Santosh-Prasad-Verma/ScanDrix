package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// RunTestsToolOptions configures scoped test execution.
type RunTestsToolOptions struct {
	Sandbox SandboxExecutor
}

// NewRunTestsTool creates the scoped test execution tool.
func NewRunTestsTool(opts RunTestsToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "runTests",
		ToolDesc: "Run unit tests for a specific package, file, or test function. Auto-detects test runner (go test, pytest, npm test, cargo test). " +
			"Use this to verify whether a suspected bug reproduces under an existing test or to validate a regression hypothesis.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Path to test file or package directory (e.g. 'internal/review/orchestrator_test.go' or 'tests/unit')",
				},
				"testName": {
					Type:        "string",
					Description: "Optional test function name or regex pattern (e.g. 'TestOrchestrator_Deduplication')",
				},
			},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "file", "target"))
			testName := StringArg(input, "testName", "test", "name", "pattern")

			if opts.Sandbox == nil {
				return contracts.ToolResult{
					Output: "⚠️ Sandbox executor unavailable — cannot run unit tests.",
				}, nil
			}

			res, err := execSandboxTests(ctx.Context, opts.Sandbox, path, testName)
			if err != nil {
				return contracts.ToolResult{
					Output:  fmt.Sprintf("Error executing tests: %v", err),
					IsError: true,
				}, nil
			}

			return contracts.ToolResult{Output: res}, nil
		},
	}
}

func execSandboxTests(ctx context.Context, sandbox SandboxExecutor, path, testName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var cmd string

	switch ext {
	case ".go":
		dir := filepath.Dir(path)
		if dir == "" || dir == "." {
			dir = "."
		} else {
			dir = "./" + dir
		}
		var nameFilter string
		if testName != "" {
			nameFilter = fmt.Sprintf(" -run %s", ShellQuote(testName))
		}
		cmd = fmt.Sprintf("go test -v%s %s 2>&1 | head -n 100", nameFilter, ShellQuote(dir))

	case ".py":
		var nameFilter string
		if testName != "" {
			nameFilter = fmt.Sprintf(" -k %s", ShellQuote(testName))
		}
		cmd = fmt.Sprintf("pytest -v%s %s 2>&1 | head -n 100", nameFilter, ShellQuote(path))

	case ".ts", ".tsx", ".js", ".jsx":
		var nameFilter string
		if testName != "" {
			nameFilter = fmt.Sprintf(" -t %s", ShellQuote(testName))
		}
		cmd = fmt.Sprintf("npm test -- %s%s 2>&1 | head -n 100", ShellQuote(path), nameFilter)

	case ".rs":
		var nameFilter string
		if testName != "" {
			nameFilter = fmt.Sprintf(" %s", ShellQuote(testName))
		}
		cmd = fmt.Sprintf("cargo test%s 2>&1 | head -n 100", nameFilter)

	default:
		// Try general go test or npm test in directory
		cmd = fmt.Sprintf("go test -v ./... 2>&1 | head -n 50")
	}

	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil {
		return "", err
	}

	out := strings.TrimSpace(stdout)
	if out == "" && strings.TrimSpace(stderr) != "" {
		out = strings.TrimSpace(stderr)
	}

	status := "PASS"
	if exitCode != 0 {
		status = fmt.Sprintf("FAIL (exit %d)", exitCode)
	}

	res := fmt.Sprintf("[%s — %s]\n%s", status, cmd, out)
	if len(res) > MaxShellOutput {
		res = TruncateWithNotice(res, MaxShellOutput, "... (truncated)")
	}
	return res, nil
}
