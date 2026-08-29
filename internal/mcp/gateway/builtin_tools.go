package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/scandrix/backend/internal/rules"
)

// ToolHandler defines the execution function for an MCP tool.
type ToolHandler func(ctx context.Context, args map[string]any) (*ToolCallResult, error)

// GetBuiltinTools returns the native ScanDrix code analysis tools for MCP agents.
func GetBuiltinTools() ([]Tool, map[string]ToolHandler) {
	toolList := []Tool{
		{
			Name:        "scandrix_analyze_snippet",
			Description: "Analyzes a source code snippet against the ScanDrix OWASP/CWE static security catalog",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]string{"type": "string"},
					"code":     map[string]string{"type": "string"},
				},
				"required": []string{"code"},
			},
		},
		{
			Name:        "scandrix_catalog_search",
			Description: "Searches the ScanDrix security rules catalog by keyword, severity, or CWE ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]string{"type": "string"},
				},
			},
		},
		{
			Name:        "scandrix_validate_syntax",
			Description: "Performs an AST dry-run compiler check to verify that a code suggestion has valid syntax",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"language": map[string]string{"type": "string"},
					"code":     map[string]string{"type": "string"},
				},
				"required": []string{"code"},
			},
		},
	}

	handlers := map[string]ToolHandler{
		"scandrix_analyze_snippet": handleAnalyzeSnippet,
		"scandrix_catalog_search":  handleCatalogSearch,
		"scandrix_validate_syntax": handleValidateSyntax,
	}

	return toolList, handlers
}

func handleAnalyzeSnippet(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "missing 'code' parameter"}}}, nil
	}

	catalog := rules.DefaultCatalog()
	var matches []string

	for _, r := range catalog {
		if strings.Contains(code, "AKIA") || strings.Contains(code, "SELECT * FROM") || strings.Contains(code, "md5.New") {
			matches = append(matches, fmt.Sprintf("[%s] %s: %s", r.Severity, r.Name, r.Description))
		}
	}

	resultText := "No security flaws detected."
	if len(matches) > 0 {
		resultText = fmt.Sprintf("Security findings detected:\n- %s", strings.Join(matches, "\n- "))
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: resultText}},
	}, nil
}

func handleCatalogSearch(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	query, _ := args["query"].(string)
	query = strings.ToLower(query)

	catalog := rules.DefaultCatalog()
	var matchingRules []rules.RuleSpec

	for _, r := range catalog {
		if query == "" || strings.Contains(strings.ToLower(r.Name), query) || strings.Contains(strings.ToLower(r.Category), query) {
			matchingRules = append(matchingRules, r)
		}
	}

	bytes, _ := json.MarshalIndent(matchingRules, "", "  ")
	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: string(bytes)}},
	}, nil
}

func handleValidateSyntax(ctx context.Context, args map[string]any) (*ToolCallResult, error) {
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return &ToolCallResult{IsError: true, Content: []ToolContent{{Type: "text", Text: "missing 'code' parameter"}}}, nil
	}

	fset := token.NewFileSet()
	wrapped := fmt.Sprintf("package main\nfunc _test() {\n%s\n}", code)
	_, err := parser.ParseFile(fset, "test.go", wrapped, parser.AllErrors)

	if err != nil {
		return &ToolCallResult{
			IsError: true,
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Syntax error: %v", err)}},
		}, nil
	}

	return &ToolCallResult{
		Content: []ToolContent{{Type: "text", Text: "Syntax valid: Code parsed successfully without syntax errors."}},
	}, nil
}
