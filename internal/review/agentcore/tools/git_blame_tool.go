package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// GitBlameToolOptions configures git blame inspection.
type GitBlameToolOptions struct {
	Git     GitClient
	Sandbox SandboxExecutor
}

// NewGitBlameTool creates the git blame inspector tool.
func NewGitBlameTool(opts GitBlameToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "gitBlame",
		ToolDesc: "Inspect git blame for a specific file and line range. Returns commit hashes, authors, commit dates, and code. " +
			"Use this to understand who wrote the original code and when recent changes were introduced.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "File path relative to repo root",
				},
				"startLine": {
					Type:        "integer",
					Description: "1-based starting line number",
				},
				"endLine": {
					Type:        "integer",
					Description: "1-based ending line number",
				},
			},
			Required: []string{"path"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "file"))
			if path == "" {
				return contracts.ToolResult{Output: "Error: path is required", IsError: true}, nil
			}

			startLine := IntArg(input, 1, "startLine", "start_line")
			endLine := IntArg(input, 0, "endLine", "end_line")

			if opts.Git != nil {
				res, err := opts.Git.Blame(ctx.Context, path, startLine, endLine)
				if err == nil && strings.TrimSpace(res) != "" {
					return contracts.ToolResult{Output: res}, nil
				}
			}

			if opts.Sandbox != nil {
				res, handled, err := execSandboxBlame(ctx.Context, opts.Sandbox, path, startLine, endLine)
				if err == nil && handled {
					return res, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("⚠️ Git blame unavailable for %s", path),
			}, nil
		},
	}
}

func execSandboxBlame(
	ctx context.Context,
	sandbox SandboxExecutor,
	path string,
	startLine, endLine int,
) (contracts.ToolResult, bool, error) {
	safePath := ShellQuote(path)
	var rangeArg string
	if startLine > 0 {
		sl := startLine
		el := endLine
		if el < sl {
			el = sl + 20
		}
		rangeArg = fmt.Sprintf(" -L %d,%d", sl, el)
	}

	cmd := fmt.Sprintf("git blame --date=short%s %s 2>/dev/null | head -n 50", rangeArg, safePath)
	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil || exitCode != 0 {
		if strings.TrimSpace(stderr) != "" {
			return contracts.ToolResult{
				Output:  fmt.Sprintf("Git blame error on %s: %s", path, strings.TrimSpace(stderr)),
				IsError: true,
			}, true, nil
		}
		return contracts.ToolResult{}, false, err
	}

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return contracts.ToolResult{Output: fmt.Sprintf("No git blame history for %s", path)}, true, nil
	}

	return contracts.ToolResult{Output: trimmed}, true, nil
}
