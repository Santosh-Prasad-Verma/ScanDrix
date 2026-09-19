package tools

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

var (
	goImportRegex = regexp.MustCompile(`import\s+(?:\(([^)]+)\)|"([^"]+)")`)
	jsImportRegex = regexp.MustCompile(`(?:import|from)\s+['"]([^'"]+)['"]|require\(['"]([^'"]+)['"]\)`)
	pyImportRegex = regexp.MustCompile(`(?:import\s+([a-zA-Z0-9_.]+)|from\s+([a-zA-Z0-9_.]+)\s+import)`)
)

// ImportGraphToolOptions configures dependency import graph mapping.
type ImportGraphToolOptions struct {
	FS      RepositoryFS
	Sandbox SandboxExecutor
}

// NewImportGraphTool creates the dependency and import graph analyzer tool.
func NewImportGraphTool(opts ImportGraphToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "importGraph",
		ToolDesc: "Analyze imports and package dependencies for a file or directory. Helps identify tight coupling, " +
			"architectural boundary violations, and potential circular dependencies.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Path to source file to inspect dependencies for",
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
					Output:  fmt.Sprintf("Could not read %s: %v", path, err),
					IsError: true,
				}, nil
			}

			imports := extractImports(content, path)
			if len(imports) == 0 {
				return contracts.ToolResult{
					Output: fmt.Sprintf("No external or package imports detected in %s.", path),
				}, nil
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Dependency import graph for %s (%d imports):\n", path, len(imports)))
			for _, imp := range imports {
				sb.WriteString(fmt.Sprintf("  → %s\n", imp))
			}

			return contracts.ToolResult{Output: sb.String()}, nil
		},
	}
}

func extractImports(content, path string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	seen := make(map[string]bool)
	res := make([]string, 0)

	addImport := func(imp string) {
		clean := strings.Trim(strings.TrimSpace(imp), `"'`)
		if clean != "" && !seen[clean] {
			seen[clean] = true
			res = append(res, clean)
		}
	}

	switch ext {
	case ".go":
		matches := goImportRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if m[2] != "" {
				addImport(m[2])
			} else if m[1] != "" {
				for _, line := range strings.Split(m[1], "\n") {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" && !strings.HasPrefix(trimmed, "//") {
						parts := strings.Fields(trimmed)
						if len(parts) > 0 {
							addImport(parts[len(parts)-1])
						}
					}
				}
			}
		}

	case ".ts", ".tsx", ".js", ".jsx":
		matches := jsImportRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if m[1] != "" {
				addImport(m[1])
			} else if m[2] != "" {
				addImport(m[2])
			}
		}

	case ".py":
		matches := pyImportRegex.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if m[1] != "" {
				addImport(m[1])
			} else if m[2] != "" {
				addImport(m[2])
			}
		}
	}

	return res
}
