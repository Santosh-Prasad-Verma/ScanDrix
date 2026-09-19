package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// GitLogToolOptions configures git commit log inspection.
type GitLogToolOptions struct {
	Git     GitClient
	Sandbox SandboxExecutor
}

// NewGitLogTool creates the git commit log inspector tool.
func NewGitLogTool(opts GitLogToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "gitLog",
		ToolDesc: "Inspect recent git commits for the repository or a specific file. Returns commit hash, author, date, and message. " +
			"Useful for understanding the intent of recent changes and identifying regressions.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Optional file or directory path to scope commit history (default: entire repo)",
				},
				"maxCommits": {
					Type:        "integer",
					Description: "Maximum number of recent commits to return (default: 10, max: 30)",
				},
			},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "file"))
			maxCommits := IntArg(input, 10, "maxCommits", "max_commits", "limit")
			if maxCommits < 1 {
				maxCommits = 1
			}
			if maxCommits > 30 {
				maxCommits = 30
			}

			if opts.Git != nil {
				res, err := opts.Git.Log(ctx.Context, path, maxCommits)
				if err == nil && strings.TrimSpace(res) != "" {
					return contracts.ToolResult{Output: res}, nil
				}
			}

			if opts.Sandbox != nil {
				res, handled, err := execSandboxLog(ctx.Context, opts.Sandbox, path, maxCommits)
				if err == nil && handled {
					return res, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("⚠️ Git log unavailable for %s", path),
			}, nil
		},
	}
}

func execSandboxLog(
	ctx context.Context,
	sandbox SandboxExecutor,
	path string,
	maxCommits int,
) (contracts.ToolResult, bool, error) {
	var pathArg string
	if path != "" && path != "." {
		pathArg = fmt.Sprintf(" -- %s", ShellQuote(path))
	}

	cmd := fmt.Sprintf("git log -n %d --oneline --decorate --stat%s 2>/dev/null", maxCommits, pathArg)
	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil || exitCode != 0 {
		if strings.TrimSpace(stderr) != "" {
			return contracts.ToolResult{
				Output:  fmt.Sprintf("Git log error: %s", strings.TrimSpace(stderr)),
				IsError: true,
			}, true, nil
		}
		return contracts.ToolResult{}, false, err
	}

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return contracts.ToolResult{Output: "No commit history found."}, true, nil
	}

	if len(trimmed) > MaxShellOutput {
		trimmed = TruncateWithNotice(trimmed, MaxShellOutput, "... (truncated)")
	}

	return contracts.ToolResult{Output: trimmed}, true, nil
}
