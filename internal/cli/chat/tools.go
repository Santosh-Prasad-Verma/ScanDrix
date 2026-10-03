// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package chat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/pentest"
	"github.com/scandrix/backend/internal/pathguard"
)

// ToolResult encapsulates the outcome of an autonomous agent tool invocation.
type ToolResult struct {
	ToolName string         `json:"tool_name"`
	Output   string         `json:"output"`
	IsError  bool           `json:"is_error"`
	Duration time.Duration  `json:"duration"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Tool represents a callable capability within the autonomous chat agent loop.
type Tool interface {
	Name() string
	Description() string
	ParametersJSON() string
	Execute(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// ToolRegistry manages registered tools and handles execution dispatch.
type ToolRegistry struct {
	workspaceRoot string
	tools         map[string]Tool
}

// NewToolRegistry initializes the tool registry with all native developer capabilities.
func NewToolRegistry(workspaceRoot string) *ToolRegistry {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	abs, err := filepath.Abs(workspaceRoot)
	if err == nil {
		workspaceRoot = abs
	}

	reg := &ToolRegistry{
		workspaceRoot: workspaceRoot,
		tools:         make(map[string]Tool),
	}

	reg.Register(&ViewFileTool{workspaceRoot: workspaceRoot})
	reg.Register(&SearchFilesTool{workspaceRoot: workspaceRoot})
	reg.Register(&ListFilesTool{workspaceRoot: workspaceRoot})
	reg.Register(&RunCommandTool{workspaceRoot: workspaceRoot})
	reg.Register(&ApplyPatchTool{workspaceRoot: workspaceRoot})
	reg.Register(&PentestTool{workspaceRoot: workspaceRoot})

	return reg
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

// Get looks up a tool by name.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns all registered tools.
func (r *ToolRegistry) All() []Tool {
	var list []Tool
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}

// Execute handles parameter parsing and tool dispatch.
func (r *ToolRegistry) Execute(ctx context.Context, name string, rawArgs json.RawMessage) (*ToolResult, error) {
	tool, ok := r.tools[name]
	if !ok {
		return &ToolResult{
			ToolName: name,
			Output:   fmt.Sprintf("Tool %q not found in registry", name),
			IsError:  true,
		}, nil
	}

	args := make(map[string]any)
	if len(rawArgs) > 0 && string(rawArgs) != "null" {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return &ToolResult{
				ToolName: name,
				Output:   fmt.Sprintf("Invalid JSON arguments for tool %q: %v", name, err),
				IsError:  true,
			}, nil
		}
	}

	start := time.Now()
	res, err := tool.Execute(ctx, args)
	if err != nil {
		return &ToolResult{
			ToolName: name,
			Output:   fmt.Sprintf("Tool execution failed: %v", err),
			IsError:  true,
			Duration: time.Since(start),
		}, nil
	}
	res.Duration = time.Since(start)
	return res, nil
}

// VIEW FILE TOOL

type ViewFileTool struct {
	workspaceRoot string
}

func (t *ViewFileTool) Name() string {
	return "view_file"
}

func (t *ViewFileTool) Description() string {
	return "Inspect contents of a file in the workspace, with optional start and end line ranges (1-indexed)."
}

func (t *ViewFileTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Relative path to file"},
			"start_line": {"type": "integer", "description": "Start line number (1-indexed)"},
			"end_line": {"type": "integer", "description": "End line number (inclusive)"}
		},
		"required": ["path"]
	}`
}

func (t *ViewFileTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	relPath, _ := args["path"].(string)
	if relPath == "" {
		return &ToolResult{ToolName: t.Name(), Output: "Missing 'path' argument", IsError: true}, nil
	}

	// relPath is supplied by the model, so it is confined to the workspace
	// before it is opened.
	fullPath, pathErr := pathguard.ResolvePath(t.workspaceRoot, relPath)
	if pathErr != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Refusing %s: %v", relPath, pathErr), IsError: true}, nil
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Failed opening file %s: %v", relPath, err), IsError: true}, nil
	}
	defer file.Close()

	startLine := 1
	if sl, ok := args["start_line"].(float64); ok && int(sl) > 0 {
		startLine = int(sl)
	}
	endLine := 0
	if el, ok := args["end_line"].(float64); ok && int(el) > 0 {
		endLine = int(el)
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	current := 0
	for scanner.Scan() {
		current++
		if current < startLine {
			continue
		}
		if endLine > 0 && current > endLine {
			break
		}
		lines = append(lines, fmt.Sprintf("%4d | %s", current, scanner.Text()))
	}

	return &ToolResult{
		ToolName: t.Name(),
		Output:   strings.Join(lines, "\n"),
		IsError:  false,
		Metadata: map[string]any{"lines_returned": len(lines), "total_lines": current},
	}, nil
}

// SEARCH FILES TOOL

type SearchFilesTool struct {
	workspaceRoot string
}

func (t *SearchFilesTool) Name() string {
	return "search_files"
}

func (t *SearchFilesTool) Description() string {
	return "Search for a regex or substring across workspace files, returning matching lines and file paths."
}

func (t *SearchFilesTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search pattern or substring"},
			"case_sensitive": {"type": "boolean", "description": "Whether search is case-sensitive"}
		},
		"required": ["query"]
	}`
}

func (t *SearchFilesTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return &ToolResult{ToolName: t.Name(), Output: "Missing 'query' argument", IsError: true}, nil
	}

	caseSensitive, _ := args["case_sensitive"].(bool)
	rePattern := regexp.QuoteMeta(query)
	if !caseSensitive {
		rePattern = "(?i)" + rePattern
	}
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Invalid regex: %v", err), IsError: true}, nil
	}

	var matches []string
	limit := 50
	walkErr := filepath.Walk(t.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				n := info.Name()
				if n == ".git" || n == "node_modules" || n == "vendor" || n == "bin" || n == "dist" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if len(matches) >= limit {
			return nil
		}

		rel, _ := filepath.Rel(t.workspaceRoot, path)
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		lineNum := 0
		for sc.Scan() {
			lineNum++
			line := sc.Text()
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, lineNum, strings.TrimSpace(line)))
				if len(matches) >= limit {
					break
				}
			}
		}
		return nil
	})

	if walkErr != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Walk error: %v", walkErr), IsError: true}, nil
	}

	if len(matches) == 0 {
		return &ToolResult{ToolName: t.Name(), Output: "No matches found.", IsError: false}, nil
	}

	return &ToolResult{
		ToolName: t.Name(),
		Output:   strings.Join(matches, "\n"),
		IsError:  false,
		Metadata: map[string]any{"matches_found": len(matches)},
	}, nil
}

// LIST FILES TOOL

type ListFilesTool struct {
	workspaceRoot string
}

func (t *ListFilesTool) Name() string {
	return "list_files"
}

func (t *ListFilesTool) Description() string {
	return "List directories and files in a workspace directory."
}

func (t *ListFilesTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"sub_path": {"type": "string", "description": "Optional subdirectory relative to workspace root"}
		}
	}`
}

func (t *ListFilesTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	sub, _ := args["sub_path"].(string)
	dir := filepath.Join(t.workspaceRoot, sub)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Failed reading directory: %v", err), IsError: true}, nil
	}

	var results []string
	for _, e := range entries {
		info, _ := e.Info()
		prefix := "  📄"
		if e.IsDir() {
			prefix = "  📁"
		}
		sizeStr := ""
		if info != nil && !e.IsDir() {
			sizeStr = fmt.Sprintf(" (%d bytes)", info.Size())
		}
		results = append(results, fmt.Sprintf("%s %s%s", prefix, e.Name(), sizeStr))
	}

	return &ToolResult{
		ToolName: t.Name(),
		Output:   strings.Join(results, "\n"),
		IsError:  false,
		Metadata: map[string]any{"count": len(entries)},
	}, nil
}

// RUN COMMAND TOOL

type RunCommandTool struct {
	workspaceRoot string
}

func (t *RunCommandTool) Name() string {
	return "run_command"
}

func (t *RunCommandTool) Description() string {
	return "Execute a safe shell command in the workspace directory (e.g., git status, go test, npm test)."
}

func (t *RunCommandTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The command line string to run"},
			"timeout_seconds": {"type": "integer", "description": "Execution timeout in seconds (default 30)"}
		},
		"required": ["command"]
	}`
}

func (t *RunCommandTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	cmdStr, _ := args["command"].(string)
	if cmdStr == "" {
		return &ToolResult{ToolName: t.Name(), Output: "Missing 'command' argument", IsError: true}, nil
	}

	// Security filter: Block dangerous destructive commands
	lower := strings.ToLower(cmdStr)
	if strings.Contains(lower, "rm -rf /") || strings.Contains(lower, ":(){ :|:& };:") || strings.Contains(lower, "mkfs") {
		return &ToolResult{
			ToolName: t.Name(),
			Output:   "Command blocked by ScanDrix security policy (destructive system command detected)",
			IsError:  true,
		}, nil
	}

	timeoutSec := 30
	if ts, ok := args["timeout_seconds"].(float64); ok && int(ts) > 0 {
		timeoutSec = int(ts)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", cmdStr)
	cmd.Dir = t.workspaceRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n--- STDERR ---\n"
		}
		output += stderr.String()
	}

	isError := err != nil
	if err != nil && output == "" {
		output = fmt.Sprintf("Command failed with error: %v", err)
	}

	return &ToolResult{
		ToolName: t.Name(),
		Output:   output,
		IsError:  isError,
	}, nil
}

// APPLY PATCH TOOL

type ApplyPatchTool struct {
	workspaceRoot string
}

func (t *ApplyPatchTool) Name() string {
	return "apply_patch"
}

func (t *ApplyPatchTool) Description() string {
	return "Apply a replacement chunk or unified diff patch to a file in the workspace."
}

func (t *ApplyPatchTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Path to file"},
			"target_content": {"type": "string", "description": "Exact lines of code to replace"},
			"replacement_content": {"type": "string", "description": "New content to replace with"}
		},
		"required": ["path", "target_content", "replacement_content"]
	}`
}

func (t *ApplyPatchTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	relPath, _ := args["path"].(string)
	targetContent, _ := args["target_content"].(string)
	replacementContent, _ := args["replacement_content"].(string)

	if relPath == "" || targetContent == "" {
		return &ToolResult{ToolName: t.Name(), Output: "Missing path or target_content", IsError: true}, nil
	}

	// Confined for the same reason as the read path above: this tool writes.
	fullPath, pathErr := pathguard.ResolvePath(t.workspaceRoot, relPath)
	if pathErr != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Refusing %s: %v", relPath, pathErr), IsError: true}, nil
	}
	contentBytes, err := os.ReadFile(fullPath)
	if err != nil {
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Failed reading file %s: %v", relPath, err), IsError: true}, nil
	}

	content := string(contentBytes)
	if !strings.Contains(content, targetContent) {
		return &ToolResult{
			ToolName: t.Name(),
			Output:   fmt.Sprintf("Target content not found in %s", relPath),
			IsError:  true,
		}, nil
	}

	newContent := strings.Replace(content, targetContent, replacementContent, 1)
	if err := os.WriteFile(fullPath, []byte(newContent), 0600); err != nil { // #nosec G703 -- fullPath comes from pathguard.ResolvePath against the workspace root
		return &ToolResult{ToolName: t.Name(), Output: fmt.Sprintf("Failed saving patched file %s: %v", relPath, err), IsError: true}, nil
	}

	return &ToolResult{
		ToolName: t.Name(),
		Output:   fmt.Sprintf("Successfully applied patch to %s", relPath),
		IsError:  false,
	}, nil
}

// PENTEST TOOL

type PentestTool struct {
	workspaceRoot string
}

func (t *PentestTool) Name() string {
	return "pentest_scan"
}

func (t *PentestTool) Description() string {
	return "Run the ScanDrix autonomous penetration test engine on a workspace or URL."
}

func (t *PentestTool) ParametersJSON() string {
	return `{
		"type": "object",
		"properties": {
			"target": {"type": "string", "description": "Target directory or HTTP endpoint (default workspace root)"},
			"mode": {"type": "string", "description": "Scan mode: quick, deep, or autonomous"}
		}
	}`
}

func (t *PentestTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	target, _ := args["target"].(string)
	if target == "" {
		target = t.workspaceRoot
	}
	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "quick"
	}

	executor := pentest.NewPentestExecutor()
	report, err := executor.ExecuteTarget(ctx, pentest.PentestTarget{
		TargetURI: target,
		Mode:      mode,
	})
	if err != nil {
		return &ToolResult{
			ToolName: t.Name(),
			Output:   fmt.Sprintf("Pentest scan failed: %v", err),
			IsError:  true,
		}, nil
	}

	var buf bytes.Buffer
	pentest.PrintTerminalReport(&buf, report)

	return &ToolResult{
		ToolName: t.Name(),
		Output:   buf.String(),
		IsError:  false,
		Metadata: map[string]any{
			"total_findings": len(report.Findings),
			"critical":       report.CriticalCount,
			"high":           report.HighCount,
			"pass":           report.PassAssessment,
		},
	}, nil
}
