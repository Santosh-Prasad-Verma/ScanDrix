package tools

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	MaxGrepMatches = 50
	MaxGrepGroups  = 40
)

// GrepToolOptions configures the grep discovery tool.
type GrepToolOptions struct {
	Sandbox    SandboxExecutor
	FS         RepositoryFS
	MaxMatches int
	MaxGroups  int
}

// NewGrepTool creates the specialized grep code discovery tool.
func NewGrepTool(opts GrepToolOptions) contracts.AgentTool {
	maxMatches := opts.MaxMatches
	if maxMatches <= 0 {
		maxMatches = MaxGrepMatches
	}
	maxGroups := opts.MaxGroups
	if maxGroups <= 0 {
		maxGroups = MaxGrepGroups
	}

	return &BaseTool{
		ToolName: "grep",
		ToolDesc: "DISCOVERY tool: search the repo for a regex pattern. Returns 'file:line:content' with context. " +
			"Use grep BEFORE readFile to locate what you need — never read a whole file to find something. " +
			"Common patterns: grep('methodName\\\\(') for callers, grep('CONSTANT') for usages, grep('implements X') for implementations. " +
			"Use namesOnly=true for blast-radius analysis (which files are affected). " +
			"Use excludeTests=true to focus on production code.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"pattern": {
					Type:        "string",
					Description: "Regex pattern to search for. Example: 'handleWebhook\\\\(' or 'maxRetries'",
				},
				"glob": {
					Type:        "string",
					Description: "Optional glob to filter files (e.g. '*.go', '*.ts', '*.py')",
				},
				"path": {
					Type:        "string",
					Description: "Optional directory or file to scope the search (default: '.')",
				},
				"namesOnly": {
					Type:        "boolean",
					Description: "Return only matching file paths instead of content lines. Useful for blast-radius checks.",
				},
				"excludeTests": {
					Type:        "boolean",
					Description: "Exclude test and spec files from results to focus on production callers.",
				},
			},
			Required: []string{"pattern"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			pattern := StringArg(input, "pattern", "query", "regex")
			if pattern == "" {
				return contracts.ToolResult{Output: "Error: pattern is required", IsError: true}, nil
			}

			glob := StringArg(input, "glob", "include")
			searchPath := NormalizePath(StringArg(input, "path", "directory", "dir"))
			if searchPath == "" {
				searchPath = "."
			}
			namesOnly := BoolArg(input, false, "namesOnly", "names_only")
			excludeTests := BoolArg(input, false, "excludeTests", "exclude_tests")

			// Check if sandbox exec is available
			if opts.Sandbox != nil {
				res, handled, err := execSandboxGrep(ctx, opts.Sandbox, pattern, glob, searchPath, namesOnly, excludeTests, maxMatches, maxGroups)
				if err == nil && handled {
					return res, nil
				}
			}

			// Fallback: RepositoryFS or internal regex search
			if opts.FS != nil {
				res, err := opts.FS.Grep(ctx.Context, pattern, searchPath)
				if err != nil {
					return contracts.ToolResult{
						Output:  fmt.Sprintf("Error searching for %q: %v", pattern, err),
						IsError: true,
					}, nil
				}
				filtered := postProcessGrepOutput(res, namesOnly, excludeTests, maxMatches, maxGroups)
				return contracts.ToolResult{Output: filtered}, nil
			}

			return contracts.ToolResult{Output: "No search executor available for grep", IsError: true}, nil
		},
	}
}

func execSandboxGrep(
	ctx contracts.ToolContext,
	sandbox SandboxExecutor,
	pattern, glob, searchPath string,
	namesOnly, excludeTests bool,
	maxMatches, maxGroups int,
) (contracts.ToolResult, bool, error) {
	safePattern := ShellQuote(pattern)
	safePath := ShellQuote(searchPath)

	var globArg string
	if glob != "" {
		globArg = fmt.Sprintf(" --glob %s", ShellQuote(glob))
	}

	var excludeTestsArg string
	if excludeTests {
		excludeTestsArg = " --glob '!*test*' --glob '!*Test*' --glob '!*spec*' --glob '!*Spec*' --glob '!*__tests__*'"
	}

	modeArg := " -n -C 2"
	if namesOnly {
		modeArg = " -l"
	}

	cmd := fmt.Sprintf("rg %s%s%s%s %s", safePattern, globArg, excludeTestsArg, modeArg, safePath)
	stdout, _, exitCode, err := sandbox.Exec(ctx.Context, cmd)
	if err != nil {
		return contracts.ToolResult{}, false, err
	}

	// Exit code 1 means no match found (standard ripgrep behavior)
	if exitCode == 1 || strings.TrimSpace(stdout) == "" {
		return contracts.ToolResult{Output: "No matches found."}, true, nil
	}

	// Exit code >= 2 indicates invalid regex or execution error
	if exitCode >= 2 {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("ripgrep error searching for %q: invalid pattern or search error", pattern),
			IsError: true,
		}, true, nil
	}

	raw := strings.TrimSpace(stdout)
	if namesOnly {
		lines := strings.Split(raw, "\n")
		if len(lines) > maxMatches {
			return contracts.ToolResult{
				Output: strings.Join(lines[:maxMatches], "\n") + fmt.Sprintf("\n... (%d more files hidden)", len(lines)-maxMatches),
			}, true, nil
		}
		return contracts.ToolResult{Output: raw}, true, nil
	}

	// Format match groups separated by --
	groups := strings.Split(raw, "\n--\n")
	if len(groups) > maxGroups {
		trimmed := strings.Join(groups[:maxGroups], "\n--\n")
		notice := fmt.Sprintf("\n... (%d more match groups hidden — narrow with a more specific regex or path)", len(groups)-maxGroups)
		return contracts.ToolResult{Output: trimmed + notice}, true, nil
	}

	return contracts.ToolResult{Output: raw}, true, nil
}

func postProcessGrepOutput(raw string, namesOnly, excludeTests bool, maxMatches, maxGroups int) string {
	if strings.TrimSpace(raw) == "" {
		return "No matches found."
	}

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	filteredLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if excludeTests {
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "test") || strings.Contains(lower, "spec") || strings.Contains(lower, "__tests__") {
				continue
			}
		}
		filteredLines = append(filteredLines, trimmed)
	}

	if len(filteredLines) == 0 {
		return "No matches found."
	}

	if namesOnly {
		seen := make(map[string]bool)
		files := make([]string, 0)
		for _, l := range filteredLines {
			parts := strings.SplitN(l, ":", 2)
			file := parts[0]
			if !seen[file] {
				seen[file] = true
				files = append(files, file)
				if len(files) >= maxMatches {
					break
				}
			}
		}
		return strings.Join(files, "\n")
	}

	if len(filteredLines) > maxMatches {
		return strings.Join(filteredLines[:maxMatches], "\n") +
			fmt.Sprintf("\n... (%d more matches hidden — narrow with a more specific regex or path)", len(filteredLines)-maxMatches)
	}

	return strings.Join(filteredLines, "\n")
}
