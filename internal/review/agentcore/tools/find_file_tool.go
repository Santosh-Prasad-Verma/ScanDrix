package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const MaxFindFilesLimit = 30

// FindFileToolOptions configures file finding.
type FindFileToolOptions struct {
	Sandbox  SandboxExecutor
	FS       RepositoryFS
	MaxLimit int
}

// NewFindFileTool creates the specialized findFile tool.
func NewFindFileTool(opts FindFileToolOptions) contracts.AgentTool {
	limit := opts.MaxLimit
	if limit <= 0 {
		limit = MaxFindFilesLimit
	}

	return &BaseTool{
		ToolName: "findFile",
		ToolDesc: "Find files by name or glob pattern. Use this to locate files before reading them. Supports extension filtering.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"pattern": {
					Type:        "string",
					Description: "File name or glob pattern (e.g. 'orchestrator.go', '*.test.ts', 'config')",
				},
				"path": {
					Type:        "string",
					Description: "Directory to search in (default: '.')",
				},
				"extension": {
					Type:        "string",
					Description: "Filter by extension without dot (e.g. 'go', 'ts', 'py')",
				},
			},
			Required: []string{"pattern"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			pattern := StringArg(input, "pattern", "glob", "name")
			if pattern == "" {
				return contracts.ToolResult{Output: "Error: pattern is required", IsError: true}, nil
			}

			searchPath := NormalizePath(StringArg(input, "path", "dir", "directory"))
			if searchPath == "" {
				searchPath = "."
			}
			extension := strings.TrimPrefix(StringArg(input, "extension", "ext"), ".")

			// Sandbox exec with fd or find
			if opts.Sandbox != nil {
				res, handled, err := execSandboxFindFile(ctx.Context, opts.Sandbox, pattern, searchPath, extension, limit)
				if err == nil && handled {
					return res, nil
				}
			}

			// Fallback: RepositoryFS ListDir recursive match
			if opts.FS != nil {
				entries, err := opts.FS.ListDir(ctx.Context, searchPath)
				if err != nil {
					return contracts.ToolResult{
						Output:  fmt.Sprintf("Error finding files in %s: %v", searchPath, err),
						IsError: true,
					}, nil
				}

				matching := make([]string, 0)
				patLower := strings.ToLower(pattern)
				for _, entry := range entries {
					base := strings.ToLower(filepath.Base(entry))
					if strings.Contains(base, patLower) {
						if extension != "" && !strings.HasSuffix(base, "."+strings.ToLower(extension)) {
							continue
						}
						matching = append(matching, entry)
						if len(matching) >= limit {
							break
						}
					}
				}

				if len(matching) == 0 {
					return contracts.ToolResult{
						Output: fmt.Sprintf("No files matching %q in %s", pattern, searchPath),
					}, nil
				}

				return contracts.ToolResult{Output: strings.Join(matching, "\n")}, nil
			}

			return contracts.ToolResult{Output: "No find file provider available", IsError: true}, nil
		},
	}
}

func execSandboxFindFile(
	ctx context.Context,
	sandbox SandboxExecutor,
	pattern, searchPath, extension string,
	limit int,
) (contracts.ToolResult, bool, error) {
	globPattern := pattern
	if !strings.Contains(globPattern, "*") && !strings.Contains(globPattern, "?") {
		globPattern = "*" + globPattern + "*"
	}

	var extArg string
	if extension != "" {
		extArg = fmt.Sprintf(" -e %s", ShellQuote(extension))
	}

	// Try fd first
	fdCmd := fmt.Sprintf("fd --glob %s%s %s --type f --max-results %d 2>/dev/null",
		ShellQuote(globPattern), extArg, ShellQuote(searchPath), limit)
	stdout, _, exitCode, err := sandbox.Exec(ctx, fdCmd)
	if err == nil && exitCode == 0 && strings.TrimSpace(stdout) != "" {
		return contracts.ToolResult{Output: strings.TrimSpace(stdout)}, true, nil
	}

	// Fallback to find
	var extFilter string
	if extension != "" {
		extFilter = fmt.Sprintf(" -name '*.%s'", extension)
	}
	findCmd := fmt.Sprintf("find %s -type f -iname %s%s 2>/dev/null | head -n %d",
		ShellQuote(searchPath), ShellQuote(globPattern), extFilter, limit)
	stdout, _, exitCode, err = sandbox.Exec(ctx, findCmd)
	if err == nil && exitCode == 0 && strings.TrimSpace(stdout) != "" {
		return contracts.ToolResult{Output: strings.TrimSpace(stdout)}, true, nil
	}

	return contracts.ToolResult{
		Output: fmt.Sprintf("No files matching %q in %s", pattern, searchPath),
	}, true, nil
}
