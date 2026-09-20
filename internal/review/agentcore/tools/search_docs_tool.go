package tools

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const MaxDocsOutputLength = 6000

// SearchDocsToolOptions configures external documentation searching.
type SearchDocsToolOptions struct {
	Adapter DocumentationSearchAdapter
	Docs    map[string]string // Embedded docs fallback
	MaxLen  int
}

// NewSearchDocsTool creates the external documentation search tool.
func NewSearchDocsTool(opts SearchDocsToolOptions) contracts.AgentTool {
	maxLen := opts.MaxLen
	if maxLen <= 0 {
		maxLen = MaxDocsOutputLength
	}

	return &BaseTool{
		ToolName: "searchDocs",
		ToolDesc: "VERIFY tool: search EXTERNAL package/library documentation when a finding hinges on framework behavior " +
			"you cannot verify with grep/readFile (e.g. TypeORM subQuery semantics, React Suspense boundaries, Express middleware ordering). " +
			"Returns official documentation snippets. Use ONLY when the suspected bug is about third-party library behavior.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"packageName": {
					Type:        "string",
					Description: "Package or library name as published (e.g. 'express', 'react', 'typeorm', 'gorm.io/gorm')",
				},
				"query": {
					Type:        "string",
					Description: "Specific question about library API or contract (e.g. 'subQuery returns only first row', 'db transaction rollback on panic')",
				},
			},
			Required: []string{"packageName", "query"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			packageName := StringArg(input, "packageName", "package", "library")
			query := StringArg(input, "query", "question")

			if packageName == "" || query == "" {
				return contracts.ToolResult{Output: "Error: both packageName and query are required", IsError: true}, nil
			}

			// 1. Check live adapter if available
			if opts.Adapter != nil {
				snippets, err := opts.Adapter.Search(ctx.Context, packageName, query)
				if err == nil && len(snippets) > 0 {
					var sb strings.Builder
					for i, s := range snippets {
						if i > 0 {
							sb.WriteString("\n\n---\n\n")
						}
						sb.WriteString(fmt.Sprintf("### %s\n**URL:** %s\n**Query:** %s\n\n%s", s.Title, s.URL, s.Query, s.Snippet))
					}
					out := sb.String()
					if len(out) > maxLen {
						notice := fmt.Sprintf("... (truncated — %d chars total across %d doc snippets)", len(out), len(snippets))
						out = TruncateWithNotice(out, maxLen, notice)
					}
					return contracts.ToolResult{Output: out}, nil
				}
			}

			// 2. Check embedded docs fallback if provided
			if len(opts.Docs) > 0 {
				key := strings.ToLower(packageName)
				if doc, ok := opts.Docs[key]; ok {
					out := fmt.Sprintf("### Documentation for %s\n\n%s", packageName, doc)
					if len(out) > maxLen {
						out = TruncateWithNotice(out, maxLen, "... (truncated)")
					}
					return contracts.ToolResult{Output: out}, nil
				}
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("No documentation found for %q with query %q. Verify behavior directly by reading implementation with grep/readFile.",
					packageName, query),
			}, nil
		},
	}
}
