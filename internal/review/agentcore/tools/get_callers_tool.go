package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CallGraphBlock represents a function and its callers/callees in the AST graph.
type CallGraphBlock struct {
	Header  string
	Callers []string
	Callees []string
}

// GetCallersToolOptions configures cross-file caller and callee inspection.
type GetCallersToolOptions struct {
	CallGraphData string
	FS            RepositoryFS
	Sandbox       SandboxExecutor
}

// NewGetCallersTool creates the cross-file AST caller and callee inspection tool.
func NewGetCallersTool(opts GetCallersToolOptions) contracts.AgentTool {
	blocks := parseCallGraph(opts.CallGraphData)

	return &BaseTool{
		ToolName: "getCallers",
		ToolDesc: "CROSS-FILE tool: look up who CALLS a changed function and what it CALLS (callers ← and callees →) " +
			"from the precomputed AST call graph. Use this for EVERY changed function to check cross-file impact — " +
			"who breaks if the signature or behavior changes, and which dependencies it relies on. " +
			"More precise than grep for caller lookup.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"functionName": {
					Type:        "string",
					Description: "Function or method name to look up callers/callees for (e.g. 'ProcessRefund' or 'validateOrder')",
				},
				"filePath": {
					Type:        "string",
					Description: "Optional file path to disambiguate identical function names across files",
				},
			},
			Required: []string{"functionName"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			fnName := strings.TrimSpace(StringArg(input, "functionName", "function", "symbol", "name"))
			if fnName == "" {
				return contracts.ToolResult{Output: "Error: functionName is required", IsError: true}, nil
			}
			filePath := NormalizePath(StringArg(input, "filePath", "path", "file"))

			// 1. Search in precomputed AST blocks if available
			if len(blocks) > 0 {
				hits := searchCallGraphBlocks(blocks, fnName, filePath)
				if len(hits) > 0 {
					var sb strings.Builder
					for i, hit := range hits {
						if i > 0 {
							sb.WriteString("\n\n")
						}
						sb.WriteString(hit.Header)
						if len(hit.Callers) == 0 && len(hit.Callees) == 0 {
							sb.WriteString("\n  (no callers or callees recorded)")
							continue
						}
						for _, caller := range hit.Callers {
							sb.WriteString("\n  ← " + caller)
						}
						for _, callee := range hit.Callees {
							sb.WriteString("\n  → " + callee)
						}
					}
					return contracts.ToolResult{Output: sb.String()}, nil
				}
			}

			// 2. Fallback to RepositoryFS.GetCallers if provided
			if opts.FS != nil {
				res, err := opts.FS.GetCallers(ctx.Context, fnName, filePath)
				if err == nil && strings.TrimSpace(res) != "" {
					return contracts.ToolResult{Output: res}, nil
				}
			}

			// 3. Fallback: Grep for call sites using Sandbox
			if opts.Sandbox != nil {
				grepRes, handled, _ := execGrepCallers(ctx.Context, opts.Sandbox, fnName)
				if handled {
					return grepRes, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("No call graph data found for %q. Use grep(%q) to find references across files.",
					fnName, fnName+"("),
			}, nil
		},
	}
}

func parseCallGraph(raw string) []CallGraphBlock {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	blocks := make([]CallGraphBlock, 0)
	var current *CallGraphBlock

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "←") || strings.HasPrefix(trimmed, "<-") {
			if current != nil {
				caller := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "←"), "<-"))
				current.Callers = append(current.Callers, caller)
			}
		} else if strings.HasPrefix(trimmed, "→") || strings.HasPrefix(trimmed, "->") {
			if current != nil {
				callee := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "→"), "->"))
				current.Callees = append(current.Callees, callee)
			}
		} else if !strings.HasPrefix(trimmed, "(") {
			if current != nil {
				blocks = append(blocks, *current)
			}
			current = &CallGraphBlock{
				Header:  trimmed,
				Callers: make([]string, 0),
				Callees: make([]string, 0),
			}
		}
	}

	if current != nil {
		blocks = append(blocks, *current)
	}

	return blocks
}

func searchCallGraphBlocks(blocks []CallGraphBlock, fnName, filePath string) []CallGraphBlock {
	q := strings.ToLower(fnName)
	fp := strings.ToLower(filePath)
	matches := make([]CallGraphBlock, 0)

	for _, b := range blocks {
		hLower := strings.ToLower(b.Header)
		if strings.Contains(hLower, q) {
			if fp != "" && !strings.Contains(hLower, fp) {
				continue
			}
			matches = append(matches, b)
			if len(matches) >= 5 {
				break
			}
		}
	}
	return matches
}

func execGrepCallers(ctx context.Context, sandbox SandboxExecutor, fnName string) (contracts.ToolResult, bool, error) {
	pattern := fmt.Sprintf(`%s\(`, fnName)
	cmd := fmt.Sprintf("rg '%s' -n -C 1 2>/dev/null | head -n 30", pattern)
	stdout, _, exitCode, err := sandbox.Exec(ctx, cmd)
	if err == nil && exitCode == 0 && strings.TrimSpace(stdout) != "" {
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		res := fmt.Sprintf("Callers found via search for %s():\n%s", fnName, strings.Join(lines, "\n"))
		return contracts.ToolResult{Output: res}, true, nil
	}
	return contracts.ToolResult{}, false, nil
}
