// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Tools Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package tools

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
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

// BaseNativeTool implements contracts.AgentTool.
type BaseNativeTool struct {
	ToolName    string
	ToolDesc    string
	Schema      contracts.JSONSchema
	IsStrict    bool
	ExecHandler func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error)
}

func (b *BaseNativeTool) Name() string                        { return b.ToolName }
func (b *BaseNativeTool) Description() string                 { return b.ToolDesc }
func (b *BaseNativeTool) InputSchema() contracts.JSONSchema   { return b.Schema }
func (b *BaseNativeTool) Strict() bool                        { return b.IsStrict }
func (b *BaseNativeTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	return b.ExecHandler(ctx, input)
}

// GrepMatch describes an individual search result.
type GrepMatch struct {
	Path       string `json:"path"`
	LineNumber int    `json:"line_number"`
	LineText   string `json:"line_text"`
}

// BuildGrepTool constructs the repository regex search tool.
func BuildGrepTool(rootDir string) contracts.AgentTool {
	return &BaseNativeTool{
		ToolName: "grep",
		ToolDesc: "Search for a regex pattern in the repository. Returns matching lines with file path and line number. Use to find symbols, references, configuration entries, or any text occurrence.",
		IsStrict: true,
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"pattern": {
					Type:        "string",
					Description: "Regex pattern to search for. Case-sensitive.",
				},
				"path": {
					Type:        "string",
					Description: "Optional sub-path to scope the search (relative to repo root). Defaults to current directory.",
				},
				"glob": {
					Type:        "string",
					Description: "Optional glob filter (e.g. '*.go', '*.ts') to restrict file types.",
				},
			},
			Required: []string{"pattern"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			var args struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
				Glob    string `json:"glob"`
			}
			rawBytes, _ := json.Marshal(input)
			if err := json.Unmarshal(rawBytes, &args); err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("invalid input: %v", err), IsError: true}, nil
			}

			if args.Pattern == "" {
				return contracts.ToolResult{Output: "pattern is required", IsError: true}, nil
			}

			re, err := regexp.Compile(args.Pattern)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("invalid regex pattern: %v", err), IsError: true}, nil
			}

			targetDir := rootDir
			if args.Path != "" && args.Path != "." {
				targetDir = filepath.Join(rootDir, filepath.Clean(args.Path))
			}

			var matches []GrepMatch
			maxMatches := 100
			if envMax := os.Getenv("SCANDRIX_AGENT_GREP_MAX_MATCHES"); envMax != "" {
				if parsed, err := strconv.Atoi(envMax); err == nil && parsed > 0 {
					maxMatches = parsed
				}
			}

			err = filepath.Walk(targetDir, func(p string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info.IsDir() {
					if info != nil && info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == "vendor") {
						return filepath.SkipDir
					}
					return nil
				}

				if args.Glob != "" {
					matched, _ := filepath.Match(args.Glob, filepath.Base(p))
					if !matched {
						return nil
					}
				}

				file, err := os.Open(p)
				if err != nil {
					return nil
				}
				defer file.Close()

				relPath, _ := filepath.Rel(rootDir, p)
				scanner := bufio.NewScanner(file)
				lineNo := 1
				for scanner.Scan() {
					line := scanner.Text()
					if re.MatchString(line) {
						matches = append(matches, GrepMatch{
							Path:       relPath,
							LineNumber: lineNo,
							LineText:   strings.TrimSpace(line),
						})
						if len(matches) >= maxMatches {
							return fmt.Errorf("reached match ceiling")
						}
					}
					lineNo++
				}
				return nil
			})

			outJSON, _ := json.Marshal(map[string]any{"matches": matches})
			return contracts.ToolResult{Output: string(outJSON)}, nil
		},
	}
}

// BuildReadFileTool constructs the line-bounded file reader.
func BuildReadFileTool(rootDir string) contracts.AgentTool {
	return &BaseNativeTool{
		ToolName: "readFile",
		ToolDesc: "Read a file from the repository between two line numbers (1-indexed, inclusive). Use to inspect implementation details after grep.",
		IsStrict: true,
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Repo-relative path to the file.",
				},
				"start": {
					Type:        "integer",
					Description: "First line to return (1-indexed, inclusive).",
				},
				"end": {
					Type:        "integer",
					Description: "Last line to return (1-indexed, inclusive).",
				},
			},
			Required: []string{"path", "start", "end"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			var args struct {
				Path  string `json:"path"`
				Start int    `json:"start"`
				End   int    `json:"end"`
			}
			rawBytes, _ := json.Marshal(input)
			if err := json.Unmarshal(rawBytes, &args); err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("invalid input: %v", err), IsError: true}, nil
			}

			if args.Path == "" {
				return contracts.ToolResult{Output: "path is required", IsError: true}, nil
			}
			if args.Start <= 0 {
				args.Start = 1
			}
			if args.End < args.Start {
				args.End = args.Start + 100
			}

			fullPath := filepath.Join(rootDir, filepath.Clean(args.Path))
			file, err := os.Open(fullPath)
			if err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("failed to read file: %v", err), IsError: true}, nil
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			currentLine := 1
			var builder strings.Builder

			for scanner.Scan() {
				if currentLine >= args.Start && currentLine <= args.End {
					builder.WriteString(fmt.Sprintf("%4d | %s\n", currentLine, scanner.Text()))
				}
				if currentLine > args.End {
					break
				}
				currentLine++
			}

			resJSON, _ := json.Marshal(map[string]string{"content": builder.String()})
			return contracts.ToolResult{Output: string(resJSON)}, nil
		},
	}
}

// BuildListDirTool constructs the directory listing tool.
func BuildListDirTool(rootDir string) contracts.AgentTool {
	return &BaseNativeTool{
		ToolName: "listDir",
		ToolDesc: "List files and folders under a directory in the repository, up to a maximum depth. Use to explore unfamiliar projects.",
		IsStrict: true,
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"path": {
					Type:        "string",
					Description: "Repo-relative directory path.",
				},
				"maxDepth": {
					Type:        "integer",
					Description: "Maximum recursion depth. Default 2.",
				},
			},
			Required: []string{"path"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			var args struct {
				Path     string `json:"path"`
				MaxDepth int    `json:"maxDepth"`
			}
			rawBytes, _ := json.Marshal(input)
			_ = json.Unmarshal(rawBytes, &args)

			if args.MaxDepth <= 0 {
				args.MaxDepth = 2
			}
			if args.MaxDepth > 10 {
				args.MaxDepth = 10
			}

			targetDir := rootDir
			if args.Path != "" && args.Path != "." {
				targetDir = filepath.Join(rootDir, filepath.Clean(args.Path))
			}

			var listings []string
			baseDepth := len(strings.Split(filepath.Clean(targetDir), string(filepath.Separator)))

			_ = filepath.Walk(targetDir, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() && (info.Name() == ".git" || info.Name() == "node_modules" || info.Name() == "vendor") {
					return filepath.SkipDir
				}
				currentDepth := len(strings.Split(filepath.Clean(p), string(filepath.Separator))) - baseDepth
				if currentDepth > args.MaxDepth {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				rel, _ := filepath.Rel(rootDir, p)
				if rel != "." {
					if info.IsDir() {
						listings = append(listings, rel+"/")
					} else {
						listings = append(listings, rel)
					}
				}
				return nil
			})

			resJSON, _ := json.Marshal(map[string]any{"listing": listings})
			return contracts.ToolResult{Output: string(resJSON)}, nil
		},
	}
}

// BuildExecTool constructs the read-only shell command execution sandbox.
func BuildExecTool(rootDir string) contracts.AgentTool {
	return &BaseNativeTool{
		ToolName: "exec",
		ToolDesc: "Run a read-only shell command inside the sandbox. Use sparingly for ad-hoc read-only inspection (git log, cat, git diff).",
		IsStrict: true,
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"command": {
					Type:        "string",
					Description: "Shell command to run. Must be read-only (no writes, no network calls outside the sandbox).",
				},
			},
			Required: []string{"command"},
		},
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			var args struct {
				Command string `json:"command"`
			}
			rawBytes, _ := json.Marshal(input)
			_ = json.Unmarshal(rawBytes, &args)

			cmdStr := strings.TrimSpace(args.Command)
			if cmdStr == "" {
				return contracts.ToolResult{Output: "command is required", IsError: true}, nil
			}

			// Security guard: forbid mutating / destructive commands
			disallowedPrefixes := []string{"rm", "mv", "touch", "mkdir", "curl", "wget", "chmod", "chown", "dd", "mkfs"}
			for _, dis := range disallowedPrefixes {
				if strings.HasPrefix(cmdStr, dis+" ") || cmdStr == dis {
					return contracts.ToolResult{
						Output:  fmt.Sprintf("command disallowed in read-only sandbox: %s", dis),
						IsError: true,
					}, nil
				}
			}

			timeoutSec := 5
			if envTimeout := os.Getenv("SCANDRIX_AGENT_EXEC_TIMEOUT_SECONDS"); envTimeout != "" {
				if parsed, err := strconv.Atoi(envTimeout); err == nil && parsed > 0 {
					timeoutSec = parsed
				}
			}

			execCtx, cancel := context.WithTimeout(ctx.Context, time.Duration(timeoutSec)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(execCtx, "sh", "-c", cmdStr)
			cmd.Dir = rootDir

			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf

			err := cmd.Run()
			if err != nil {
				return contracts.ToolResult{
					Output:  fmt.Sprintf("error executing command: %v\nstderr: %s", err, errBuf.String()),
					IsError: true,
				}, nil
			}

			return contracts.ToolResult{
				Output: outBuf.String(),
			}, nil
		},
	}
}

// BuildNativeToolsRegistry packs grep, readFile, listDir, and exec into an InMemoryToolRegistry.
func BuildNativeToolsRegistry(rootDir string) contracts.ToolRegistry {
	return tools.NewInMemoryToolRegistry(
		BuildGrepTool(rootDir),
		BuildReadFileTool(rootDir),
		BuildListDirTool(rootDir),
		BuildExecTool(rootDir),
	)
}
