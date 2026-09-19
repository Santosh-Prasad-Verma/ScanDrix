package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// GitDiffRangeToolOptions configures git diff range inspection.
type GitDiffRangeToolOptions struct {
	Git     GitClient
	Sandbox SandboxExecutor
}

// NewGitDiffRangeTool creates the git diff range comparison tool.
func NewGitDiffRangeTool(opts GitDiffRangeToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "gitDiffRange",
		ToolDesc: "Inspect targeted git diff between two branches or commit hashes. Useful for checking incremental changes, " +
			"merge bases, or comparing a branch with main/origin.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"baseRef": {
					Type:        "string",
					Description: "Base branch or commit hash (e.g. 'main', 'HEAD~1')",
				},
				"headRef": {
					Type:        "string",
					Description: "Head branch or commit hash (e.g. 'HEAD', 'feature-branch')",
				},
				"path": {
					Type:        "string",
					Description: "Optional file path to limit diff",
				},
			},
			Required: []string{"baseRef"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			baseRef := StringArg(input, "baseRef", "base", "from")
			headRef := StringArg(input, "headRef", "head", "to")
			if headRef == "" {
				headRef = "HEAD"
			}
			path := NormalizePath(StringArg(input, "path", "file"))

			if baseRef == "" {
				return contracts.ToolResult{Output: "Error: baseRef is required", IsError: true}, nil
			}

			if opts.Git != nil {
				res, err := opts.Git.DiffRange(ctx.Context, baseRef, headRef, path)
				if err == nil && strings.TrimSpace(res) != "" {
					return contracts.ToolResult{Output: res}, nil
				}
			}

			if opts.Sandbox != nil {
				res, handled, err := execSandboxDiffRange(ctx.Context, opts.Sandbox, baseRef, headRef, path)
				if err == nil && handled {
					return res, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("⚠️ Git diff range unavailable between %s and %s", baseRef, headRef),
			}, nil
		},
	}
}

func execSandboxDiffRange(
	ctx context.Context,
	sandbox SandboxExecutor,
	baseRef, headRef, path string,
) (contracts.ToolResult, bool, error) {
	var pathArg string
	if path != "" && path != "." {
		pathArg = fmt.Sprintf(" -- %s", ShellQuote(path))
	}

	cmd := fmt.Sprintf("git diff %s...%s%s 2>/dev/null | head -n 300", ShellQuote(baseRef), ShellQuote(headRef), pathArg)
	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil || exitCode != 0 {
		if strings.TrimSpace(stderr) != "" {
			return contracts.ToolResult{
				Output:  fmt.Sprintf("Git diff range error: %s", strings.TrimSpace(stderr)),
				IsError: true,
			}, true, nil
		}
		return contracts.ToolResult{}, false, err
	}

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return contracts.ToolResult{Output: fmt.Sprintf("No diff found between %s and %s.", baseRef, headRef)}, true, nil
	}

	if len(trimmed) > MaxShellOutput {
		trimmed = TruncateWithNotice(trimmed, MaxShellOutput, "... (truncated)")
	}

	return contracts.ToolResult{Output: trimmed}, true, nil
}
