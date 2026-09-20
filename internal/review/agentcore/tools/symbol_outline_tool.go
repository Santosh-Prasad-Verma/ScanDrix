package tools

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// SymbolOutlineToolOptions configures structural outline extraction.
type SymbolOutlineToolOptions struct {
	FS      RepositoryFS
	Sandbox SandboxExecutor
}

// NewSymbolOutlineTool creates the structural symbol outline tool.
func NewSymbolOutlineTool(opts SymbolOutlineToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "symbolOutline",
		ToolDesc: "Extract structural symbols (functions, methods, types, interfaces, classes, structs) with line numbers from a file. " +
			"Use this on large files to see their structure without reading thousands of lines.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Path to file to outline (e.g. 'internal/review/orchestrator.go')",
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

			var content string
			var err error

			if opts.FS != nil {
				content, err = opts.FS.ReadFile(ctx.Context, path, 0, 0)
			} else if opts.Sandbox != nil {
				cmd := fmt.Sprintf("cat %s 2>/dev/null", ShellQuote(path))
				stdout, _, exitCode, e := opts.Sandbox.Exec(ctx.Context, cmd)
				if e == nil && exitCode == 0 {
					content = stdout
				} else {
					err = e
				}
			}

			if err != nil || strings.TrimSpace(content) == "" {
				return contracts.ToolResult{
					Output:  fmt.Sprintf("Could not read %s to extract outline: %v", path, err),
					IsError: true,
				}, nil
			}

			lines := strings.Split(content, "\n")
			outline := ExtractOutlineFromLines(lines, path)
			header := fmt.Sprintf("Symbol outline for %s (%d lines):\n", path, len(lines))
			return contracts.ToolResult{Output: header + outline}, nil
		},
	}
}
