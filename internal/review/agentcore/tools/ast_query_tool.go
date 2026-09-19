package tools

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// AstQueryToolOptions configures AST structural queries.
type AstQueryToolOptions struct {
	FS      RepositoryFS
	Sandbox SandboxExecutor
}

// NewAstQueryTool creates the structural AST search tool.
func NewAstQueryTool(opts AstQueryToolOptions) contracts.AgentTool {
	return &BaseTool{
		ToolName: "astQuery",
		ToolDesc: "Perform structural AST queries on source code files. Find exact declarations of types, structs, interfaces, " +
			"methods, and imported packages without regex inaccuracies.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Path to source file or directory",
				},
				"queryType": {
					Type:        "string",
					Enum:        []string{"func", "type", "struct", "interface", "import"},
					Description: "Type of AST element to query for",
				},
				"name": {
					Type:        "string",
					Description: "Optional identifier or substring to filter AST symbols",
				},
			},
			Required: []string{"path", "queryType"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "file"))
			queryType := strings.ToLower(StringArg(input, "queryType", "type", "kind"))
			name := strings.ToLower(StringArg(input, "name", "symbol", "identifier"))

			if path == "" || queryType == "" {
				return contracts.ToolResult{Output: "Error: path and queryType are required", IsError: true}, nil
			}

			// If it's a Go file, we use native go/parser and go/ast for 100% precision!
			if strings.HasSuffix(path, ".go") && opts.FS != nil {
				res, err := queryGoAST(ctx.Context, opts.FS, path, queryType, name)
				if err == nil && len(res) > 0 {
					return contracts.ToolResult{Output: strings.Join(res, "\n")}, nil
				}
			}

			// Multi-language fallback using structural grep / sandbox
			if opts.Sandbox != nil {
				res, handled, err := execSandboxAstQuery(ctx.Context, opts.Sandbox, path, queryType, name)
				if err == nil && handled {
					return res, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("No AST matches for %s %q in %s", queryType, name, path),
			}, nil
		},
	}
}

func queryGoAST(ctx context.Context, fs RepositoryFS, path, queryType, name string) ([]string, error) {
	content, err := fs.ReadFile(ctx, path, 0, 0)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	results := make([]string, 0)

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		pos := fset.Position(n.Pos())

		switch queryType {
		case "func":
			if fn, ok := n.(*ast.FuncDecl); ok {
				fnName := fn.Name.Name
				if name == "" || strings.Contains(strings.ToLower(fnName), name) {
					recv := ""
					if fn.Recv != nil && len(fn.Recv.List) > 0 {
						recv = "(receiver) "
					}
					results = append(results, fmt.Sprintf("%s:%d: func %s%s()", path, pos.Line, recv, fnName))
				}
			}
		case "type", "struct", "interface":
			if ts, ok := n.(*ast.TypeSpec); ok {
				typeName := ts.Name.Name
				if name == "" || strings.Contains(strings.ToLower(typeName), name) {
					kind := "type"
					switch ts.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}
					if queryType == "type" || queryType == kind {
						results = append(results, fmt.Sprintf("%s:%d: %s %s", path, pos.Line, kind, typeName))
					}
				}
			}
		case "import":
			if imp, ok := n.(*ast.ImportSpec); ok {
				impPath := imp.Path.Value
				if name == "" || strings.Contains(strings.ToLower(impPath), name) {
					results = append(results, fmt.Sprintf("%s:%d: import %s", path, pos.Line, impPath))
				}
			}
		}

		return true
	})

	return results, nil
}

func execSandboxAstQuery(
	ctx context.Context,
	sandbox SandboxExecutor,
	path, queryType, name string,
) (contracts.ToolResult, bool, error) {
	safePath := ShellQuote(path)
	var pattern string

	switch queryType {
	case "func":
		pattern = fmt.Sprintf(`(func|function|def|fn)\s+.*%s`, name)
	case "type", "struct", "interface":
		pattern = fmt.Sprintf(`(type|struct|interface|class)\s+.*%s`, name)
	case "import":
		pattern = fmt.Sprintf(`(import|require|from)\s+.*%s`, name)
	default:
		pattern = name
	}

	cmd := fmt.Sprintf("rg '%s' -n %s 2>/dev/null | head -n 30", pattern, safePath)
	stdout, _, exitCode, err := sandbox.Exec(ctx, cmd)
	if err == nil && exitCode == 0 && strings.TrimSpace(stdout) != "" {
		return contracts.ToolResult{Output: strings.TrimSpace(stdout)}, true, nil
	}

	return contracts.ToolResult{}, false, nil
}
