package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const (
	MaxReadLength         = 8000
	DefaultOutlineCutoff  = 500
)

// ReadFileToolOptions configures surgical file reading.
type ReadFileToolOptions struct {
	Sandbox          SandboxExecutor
	FS               RepositoryFS
	MaxReadLength    int
	OutlineFirst     bool
	OutlineThreshold int
}

// NewReadFileTool creates the specialized surgical readFile tool.
func NewReadFileTool(opts ReadFileToolOptions) contracts.AgentTool {
	maxLen := opts.MaxReadLength
	if maxLen <= 0 {
		maxLen = MaxReadLength
	}
	outlineThreshold := opts.OutlineThreshold
	if outlineThreshold <= 0 {
		outlineThreshold = DefaultOutlineCutoff
	}

	return &BaseTool{
		ToolName: "readFile",
		ToolDesc: "Read file contents with injected line numbers. Always use startLine/endLine from grep results or diff @@ markers. " +
			"Rule of thumb: readFile(file, startLine=grepLine-20, endLine=grepLine+30) gives enough context to understand callers or callees. " +
			"Only read the full file when it is small (<150 lines). Never read a whole file to find a method — grep first. " +
			"Know the exact question this range will answer before reading.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "File path relative to repo root (e.g. 'internal/review/orchestrator.go')",
				},
				"startLine": {
					Type:        "integer",
					Description: "1-based starting line number. Use around changed lines (e.g. line-20)",
				},
				"endLine": {
					Type:        "integer",
					Description: "1-based ending line number (e.g. line+30)",
				},
			},
			Required: []string{"path"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			path := NormalizePath(StringArg(input, "path", "filePath", "file"))
			if path == "" {
				return contracts.ToolResult{Output: "Error: path is required", IsError: true}, nil
			}

			startLine := IntArg(input, 0, "startLine", "start_line")
			endLine := IntArg(input, 0, "endLine", "end_line")

			// Check Sandbox execution first if available
			if opts.Sandbox != nil {
				res, handled, err := execSandboxReadFile(ctx.Context, opts.Sandbox, path, startLine, endLine, maxLen)
				if err == nil && handled {
					return res, nil
				}
			}

			// Fallback: RepositoryFS
			if opts.FS != nil {
				rawContent, err := opts.FS.ReadFile(ctx.Context, path, startLine, endLine)
				if err != nil {
					nearMisses := findNearMissFiles(ctx.Context, opts.FS, path)
					msg := fmt.Sprintf("Error reading %s: %v", path, err)
					if len(nearMisses) > 0 {
						msg += fmt.Sprintf("\nDid you mean: %s?", strings.Join(nearMisses, ", "))
					}
					return contracts.ToolResult{Output: msg, IsError: true}, nil
				}

				if strings.TrimSpace(rawContent) == "" {
					if startLine > 0 {
						return contracts.ToolResult{
							Output: fmt.Sprintf("readFile: no content in %s for requested range (startLine=%d, endLine=%d). The file likely has fewer than %d lines — re-read with smaller startLine.",
								path, startLine, endLine, startLine),
						}, nil
					}
					return contracts.ToolResult{
						Output: fmt.Sprintf("readFile: %s is empty (0 bytes).", path),
					}, nil
				}

				// Check OutlineFirst mode for full file reads without line boundaries
				if opts.OutlineFirst && startLine <= 0 && endLine <= 0 {
					lines := strings.Split(rawContent, "\n")
					if len(lines) > outlineThreshold {
						outline := ExtractOutlineFromLines(lines, path)
						msg := fmt.Sprintf("[OUTLINE-FIRST: %s has %d lines. Showing structural outline to preserve context window. Call readFile with startLine/endLine for specific sections.]\n\n%s",
							path, len(lines), outline)
						return contracts.ToolResult{Output: msg}, nil
					}
				}

				baseLine := 1
				if startLine > 0 {
					baseLine = startLine
				}
				numbered := AddLineNumbers(rawContent, baseLine)

				if len(numbered) > maxLen {
					notice := fmt.Sprintf("... (truncated — %d chars total; call readFile with a narrower range)", len(numbered))
					numbered = TruncateWithNotice(numbered, maxLen, notice)
				}

				return contracts.ToolResult{Output: numbered}, nil
			}

			return contracts.ToolResult{Output: "No file reader available", IsError: true}, nil
		},
	}
}

func execSandboxReadFile(
	ctx context.Context,
	sandbox SandboxExecutor,
	path string,
	startLine, endLine, maxLen int,
) (contracts.ToolResult, bool, error) {
	safePath := ShellQuote(path)
	var cmd string
	if startLine <= 0 && endLine <= 0 {
		cmd = fmt.Sprintf("cat %s 2>/dev/null", safePath)
	} else {
		sl := startLine
		if sl < 1 {
			sl = 1
		}
		el := endLine
		if el < sl {
			el = sl + 50
		}
		cmd = fmt.Sprintf("sed -n '%d,%dp' %s 2>/dev/null", sl, el, safePath)
	}

	stdout, stderr, exitCode, err := sandbox.Exec(ctx, cmd)
	if err != nil || exitCode != 0 {
		return contracts.ToolResult{}, false, err
	}

	if strings.TrimSpace(stdout) == "" {
		if strings.TrimSpace(stderr) != "" {
			return contracts.ToolResult{
				Output:  fmt.Sprintf("Error reading %s: %s", path, strings.TrimSpace(stderr)),
				IsError: true,
			}, true, nil
		}
		if startLine > 0 {
			return contracts.ToolResult{
				Output: fmt.Sprintf("readFile: no content in %s for requested range (startLine=%d, endLine=%d).", path, startLine, endLine),
			}, true, nil
		}
		return contracts.ToolResult{Output: fmt.Sprintf("readFile: %s is empty (0 bytes).", path)}, true, nil
	}

	baseLine := 1
	if startLine > 0 {
		baseLine = startLine
	}
	numbered := AddLineNumbers(stdout, baseLine)

	if len(numbered) > maxLen {
		notice := fmt.Sprintf("... (truncated — %d chars total; call readFile with a narrower range)", len(numbered))
		numbered = TruncateWithNotice(numbered, maxLen, notice)
	}

	return contracts.ToolResult{Output: numbered}, true, nil
}

func findNearMissFiles(ctx context.Context, fs RepositoryFS, path string) []string {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		dir = "."
	}
	targetBase := strings.ToLower(filepath.Base(path))

	entries, err := fs.ListDir(ctx, dir)
	if err != nil {
		return nil
	}

	matches := make([]string, 0, 3)
	for _, entry := range entries {
		base := strings.ToLower(filepath.Base(entry))
		if base == "" {
			continue
		}
		if strings.Contains(base, targetBase) || strings.Contains(targetBase, base) ||
			(len(base) >= 3 && len(targetBase) >= 3 && base[:3] == targetBase[:3]) {
			matches = append(matches, entry)
			if len(matches) >= 3 {
				break
			}
		}
	}
	return matches
}

// ExtractOutlineFromLines extracts structural symbols from source lines.
func ExtractOutlineFromLines(lines []string, path string) string {
	var sb strings.Builder
	ext := strings.ToLower(filepath.Ext(path))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := i + 1

		switch ext {
		case ".go":
			if strings.HasPrefix(trimmed, "func ") ||
				strings.HasPrefix(trimmed, "type ") ||
				strings.HasPrefix(trimmed, "const (") ||
				strings.HasPrefix(trimmed, "var (") {
				sb.WriteString(fmt.Sprintf("%4d: %s\n", lineNum, trimmed))
			}
		case ".ts", ".tsx", ".js", ".jsx":
			if strings.HasPrefix(trimmed, "export ") ||
				strings.HasPrefix(trimmed, "function ") ||
				strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "interface ") ||
				strings.HasPrefix(trimmed, "type ") ||
				strings.Contains(trimmed, "const ") && strings.Contains(trimmed, " = ") {
				sb.WriteString(fmt.Sprintf("%4d: %s\n", lineNum, trimmed))
			}
		case ".py":
			if strings.HasPrefix(trimmed, "def ") ||
				strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "async def ") {
				sb.WriteString(fmt.Sprintf("%4d: %s\n", lineNum, trimmed))
			}
		case ".rs":
			if strings.HasPrefix(trimmed, "pub fn ") ||
				strings.HasPrefix(trimmed, "fn ") ||
				strings.HasPrefix(trimmed, "pub struct ") ||
				strings.HasPrefix(trimmed, "pub enum ") ||
				strings.HasPrefix(trimmed, "impl ") {
				sb.WriteString(fmt.Sprintf("%4d: %s\n", lineNum, trimmed))
			}
		default:
			if strings.HasPrefix(trimmed, "func ") ||
				strings.HasPrefix(trimmed, "function ") ||
				strings.HasPrefix(trimmed, "class ") {
				sb.WriteString(fmt.Sprintf("%4d: %s\n", lineNum, trimmed))
			}
		}
	}

	res := sb.String()
	if res == "" {
		return fmt.Sprintf("No top-level functions or classes detected in %s.", path)
	}
	return res
}
