// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.

package agentcore

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

const (
	DefaultMaxFileReadBytes = 32 * 1024 // 32 KB
	DefaultOutlineThreshold = 500       // 500 lines
)

// RepositoryFS abstracts file reads and directory listings for the reviewer agent.
type RepositoryFS interface {
	ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error)
	ListDir(ctx context.Context, path string) ([]string, error)
	Grep(ctx context.Context, query, path string) (string, error)
	GetCallers(ctx context.Context, symbol, file string) (string, error)
	GitDiff(ctx context.Context, path string) (string, error)
}

// FinderToolRegistryOptions configures the tool surface exposed to review agents.
type FinderToolRegistryOptions struct {
	FS                RepositoryFS
	EnableOutline     bool
	OutlineThreshold  int
	MaxFileReadBytes  int
	DocumentationDocs map[string]string
}

// BaseAgentTool implements contracts.AgentTool.
type BaseAgentTool struct {
	ToolName    string
	ToolDesc    string
	Schema      contracts.JSONSchema
	IsStrict    bool
	ExecHandler func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error)
}

func (b *BaseAgentTool) Name() string                        { return b.ToolName }
func (b *BaseAgentTool) Description() string                 { return b.ToolDesc }
func (b *BaseAgentTool) InputSchema() contracts.JSONSchema   { return b.Schema }
func (b *BaseAgentTool) Strict() bool                        { return b.IsStrict }
func (b *BaseAgentTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	return b.ExecHandler(ctx, input)
}

// BuildFinderToolRegistry builds an enterprise ToolRegistry with caching and outline decorators.
func BuildFinderToolRegistry(opts FinderToolRegistryOptions) (contracts.ToolRegistry, *tools.ToolCallCache) {
	cache := tools.NewToolCallCache()

	maxBytes := opts.MaxFileReadBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileReadBytes
		if val := os.Getenv("SCANDRIX_MAX_FILE_READ_BYTES"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				maxBytes = parsed
			}
		}
	}

	outlineThreshold := opts.OutlineThreshold
	if outlineThreshold <= 0 {
		outlineThreshold = DefaultOutlineThreshold
		if val := os.Getenv("SCANDRIX_OUTLINE_THRESHOLD_LINES"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				outlineThreshold = parsed
			}
		}
	}

	readOnlyNavTools := map[string]bool{
		"readFile":   true,
		"listDir":    true,
		"grep":       true,
		"getCallers": true,
		"gitDiff":    true,
	}

	toolList := make([]contracts.AgentTool, 0)

	// 1. readFile tool
	readFileTool := &BaseAgentTool{
		ToolName: "readFile",
		ToolDesc: "Read file contents in the repository. Optionally provide startLine and endLine.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path":      {Type: "string", Description: "Relative file path"},
				"startLine": {Type: "integer", Description: "1-based starting line number"},
				"endLine":   {Type: "integer", Description: "1-based ending line number"},
			},
			Required: []string{"path"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			inputMap, _ := input.(map[string]any)
			path, _ := inputMap["path"].(string)
			if path == "" {
				return contracts.ToolResult{Output: "Error: path is required", IsError: true}, nil
			}

			startLine, _ := inputMap["startLine"].(int)
			endLine, _ := inputMap["endLine"].(int)

			if opts.FS == nil {
				return contracts.ToolResult{Output: "Error: repository FS unavailable", IsError: true}, nil
			}

			content, err := opts.FS.ReadFile(ctx.Context, path, startLine, endLine)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error reading file: %v", err), IsError: true}, nil
			}

			// OutlineFirst check
			if opts.EnableOutline && startLine <= 0 && endLine <= 0 {
				lines := strings.Split(content, "\n")
				if len(lines) > outlineThreshold {
					outline := generateSymbolOutline(lines, path)
					return contracts.ToolResult{
						Output: fmt.Sprintf("[OUTLINE-FIRST: File %s has %d lines. Showing structural outline to save tokens. Use startLine/endLine to read specific sections.]\n\n%s", path, len(lines), outline),
					}, nil
				}
			}

			// Size limit check
			if len(content) > maxBytes {
				content = content[:maxBytes] + fmt.Sprintf("\n... [TRUNCATED: Exceeded %d bytes limit. Specify line numbers to read specific sections.]", maxBytes)
			}

			return contracts.ToolResult{Output: content}, nil
		},
	}

	// 2. listDir tool
	listDirTool := &BaseAgentTool{
		ToolName: "listDir",
		ToolDesc: "List directory contents in the repository.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {Type: "string", Description: "Directory path relative to repository root"},
			},
			Required: []string{"path"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			inputMap, _ := input.(map[string]any)
			path, _ := inputMap["path"].(string)
			if opts.FS == nil {
				return contracts.ToolResult{Output: "Error: repository FS unavailable", IsError: true}, nil
			}

			entries, err := opts.FS.ListDir(ctx.Context, path)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error listing directory: %v", err), IsError: true}, nil
			}
			return contracts.ToolResult{Output: strings.Join(entries, "\n")}, nil
		},
	}

	// 3. grep tool
	grepTool := &BaseAgentTool{
		ToolName: "grep",
		ToolDesc: "Search across repository files using ripgrep pattern.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"query": {Type: "string", Description: "Search query or regex"},
				"path":  {Type: "string", Description: "Subdirectory or file filter"},
			},
			Required: []string{"query"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			inputMap, _ := input.(map[string]any)
			query, _ := inputMap["query"].(string)
			path, _ := inputMap["path"].(string)
			if opts.FS == nil {
				return contracts.ToolResult{Output: "Error: repository FS unavailable", IsError: true}, nil
			}

			matches, err := opts.FS.Grep(ctx.Context, query, path)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error running grep: %v", err), IsError: true}, nil
			}
			return contracts.ToolResult{Output: matches}, nil
		},
	}

	// 4. getCallers tool
	callersTool := &BaseAgentTool{
		ToolName: "getCallers",
		ToolDesc: "Find caller references to a function or symbol across the codebase.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"symbol": {Type: "string", Description: "Name of function or symbol"},
				"file":   {Type: "string", Description: "File where symbol is defined"},
			},
			Required: []string{"symbol"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			inputMap, _ := input.(map[string]any)
			symbol, _ := inputMap["symbol"].(string)
			file, _ := inputMap["file"].(string)
			if opts.FS == nil {
				return contracts.ToolResult{Output: "Error: repository FS unavailable", IsError: true}, nil
			}

			callers, err := opts.FS.GetCallers(ctx.Context, symbol, file)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error finding callers: %v", err), IsError: true}, nil
			}
			return contracts.ToolResult{Output: callers}, nil
		},
	}

	// 5. gitDiff tool
	diffTool := &BaseAgentTool{
		ToolName: "gitDiff",
		ToolDesc: "View the unified git diff for a changed file in this Pull Request.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {Type: "string", Description: "File path to view diff for"},
			},
			Required: []string{"path"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			inputMap, _ := input.(map[string]any)
			path, _ := inputMap["path"].(string)
			if opts.FS == nil {
				return contracts.ToolResult{Output: "Error: repository FS unavailable", IsError: true}, nil
			}

			diff, err := opts.FS.GitDiff(ctx.Context, path)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error retrieving diff: %v", err), IsError: true}, nil
			}
			return contracts.ToolResult{Output: diff}, nil
		},
	}

	rawTools := []contracts.AgentTool{readFileTool, listDirTool, grepTool, callersTool, diffTool}

	for _, t := range rawTools {
		if readOnlyNavTools[t.Name()] {
			toolList = append(toolList, tools.NewCachingTool(t, cache))
		} else {
			toolList = append(toolList, t)
		}
	}

	registry := tools.NewInMemoryToolRegistry(toolList...)
	return registry, cache
}

// generateSymbolOutline produces a fast structural outline of top-level functions and classes.
func generateSymbolOutline(lines []string, filename string) string {
	var outline strings.Builder
	outline.WriteString(fmt.Sprintf("Outline for %s:\n", filename))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := i + 1

		// Go, TypeScript, Python structural markers
		if strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "def ") ||
			strings.HasPrefix(trimmed, "export function ") ||
			strings.HasPrefix(trimmed, "export class ") ||
			strings.HasPrefix(trimmed, "export interface ") {
			outline.WriteString(fmt.Sprintf("  Line %4d: %s\n", lineNum, trimmed))
		}
	}

	return outline.String()
}
