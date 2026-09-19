package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	MaxListLength = 4000
	MaxListDepth  = 4
)

var StandardIgnoreDirectories = []string{
	"node_modules",
	".git",
	"dist",
	"build",
	".next",
	"__pycache__",
	"coverage",
	".turbo",
	"vendor",
	".cache",
	".gemini",
	"target",
	"bin",
	"obj",
}

// ListDirToolOptions configures directory listing.
type ListDirToolOptions struct {
	Sandbox       SandboxExecutor
	FS            RepositoryFS
	MaxListLength int
}

// NewListDirTool creates the specialized listDir tool.
func NewListDirTool(opts ListDirToolOptions) contracts.AgentTool {
	maxLen := opts.MaxListLength
	if maxLen <= 0 {
		maxLen = MaxListLength
	}

	return &BaseTool{
		ToolName: "listDir",
		ToolDesc: "List files and directories in the repository. Use maxDepth to control recursion (default: 2, max: 4). " +
			"Automatically excludes build artifacts, dependencies, and cache directories.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Directory path relative to repo root (default: '.')",
				},
				"maxDepth": {
					Type:        "integer",
					Description: "Max recursion depth (default: 2, max: 4)",
				},
			},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "directory", "dir"))
			if path == "" {
				path = "."
			}
			depth := IntArg(input, 2, "maxDepth", "max_depth", "depth")
			if depth < 1 {
				depth = 1
			}
			if depth > MaxListDepth {
				depth = MaxListDepth
			}

			// Try sandbox execution first
			if opts.Sandbox != nil {
				res, handled, err := execSandboxListDir(ctx.Context, opts.Sandbox, path, depth, maxLen)
				if err == nil && handled {
					return res, nil
				}
			}

			// Fallback: RepositoryFS
			if opts.FS != nil {
				entries, err := opts.FS.ListDir(ctx.Context, path)
				if err != nil {
					return contracts.ToolResult{
						Output:  fmt.Sprintf("Error listing directory %s: %v", path, err),
						IsError: true,
					}, nil
				}

				filtered := filterIgnoredPaths(entries)
				if len(filtered) == 0 {
					return contracts.ToolResult{Output: fmt.Sprintf("(empty directory: %s)", path)}, nil
				}

				joined := strings.Join(filtered, "\n")
				if len(joined) > maxLen {
					notice := "... (truncated — pass a more specific path or lower maxDepth to narrow listing)"
					joined = TruncateWithNotice(joined, maxLen, notice)
				}

				return contracts.ToolResult{Output: joined}, nil
			}

			return contracts.ToolResult{Output: "No directory listing provider available", IsError: true}, nil
		},
	}
}

func execSandboxListDir(
	ctx context.Context,
	sandbox SandboxExecutor,
	path string,
	depth, maxLen int,
) (contracts.ToolResult, bool, error) {
	safePath := ShellQuote(path)
	cmd := fmt.Sprintf("find %s -maxdepth %d -not -path '*/.*' 2>/dev/null | sort", safePath, depth)

	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil || exitCode != 0 {
		if strings.TrimSpace(stderr) != "" {
			return contracts.ToolResult{
				Output:  fmt.Sprintf("Error listing %s: %s", path, strings.TrimSpace(stderr)),
				IsError: true,
			}, true, nil
		}
		return contracts.ToolResult{}, false, err
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	filtered := filterIgnoredPaths(lines)
	if len(filtered) == 0 {
		return contracts.ToolResult{Output: fmt.Sprintf("(empty directory: %s)", path)}, true, nil
	}

	joined := strings.Join(filtered, "\n")
	if len(joined) > maxLen {
		notice := "... (truncated — pass a more specific path or lower maxDepth to narrow listing)"
		joined = TruncateWithNotice(joined, maxLen, notice)
	}

	return contracts.ToolResult{Output: joined}, true, nil
}

func filterIgnoredPaths(paths []string) []string {
	res := make([]string, 0, len(paths))
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" || trimmed == "." {
			continue
		}
		ignored := false
		for _, ignore := range StandardIgnoreDirectories {
			if strings.Contains(trimmed, "/"+ignore+"/") ||
				strings.HasPrefix(trimmed, ignore+"/") ||
				strings.HasSuffix(trimmed, "/"+ignore) ||
				trimmed == ignore {
				ignored = true
				break
			}
		}
		if !ignored {
			res = append(res, trimmed)
		}
	}
	return res
}
