package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// SandboxExecutor abstracts executing shell commands inside a sandbox container or local process.
type SandboxExecutor interface {
	Exec(ctx context.Context, cmd string) (stdout, stderr string, exitCode int, err error)
}

// RepositoryFS abstracts file system operations for review agents.
type RepositoryFS interface {
	ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error)
	ListDir(ctx context.Context, path string) ([]string, error)
	Grep(ctx context.Context, query, path string) (string, error)
	GetCallers(ctx context.Context, symbol, file string) (string, error)
	GitDiff(ctx context.Context, path string) (string, error)
}

// GitClient abstracts git CLI operations for git-aware review tools.
type GitClient interface {
	Blame(ctx context.Context, path string, startLine, endLine int) (string, error)
	Log(ctx context.Context, path string, maxCommits int) (string, error)
	DiffRange(ctx context.Context, baseRef, headRef, path string) (string, error)
}

// SCMReferenceFetcher abstracts fetching files from remote repositories.
type SCMReferenceFetcher interface {
	FetchFile(ctx context.Context, repo, path, ref string) (string, error)
}

// DocumentationSearchAdapter abstracts searching external library documentation.
type DocumentationSearchAdapter interface {
	Search(ctx context.Context, packageName, query string) ([]DocumentationSnippet, error)
}

// DocumentationSnippet represents a hit from external documentation search.
type DocumentationSnippet struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Query   string `json:"query"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}

// BaseTool implements contracts.AgentTool with clean idiomatic Go.
type BaseTool struct {
	ToolName    string
	ToolDesc    string
	Schema      contracts.JSONSchema
	IsStrict    bool
	ExecHandler func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error)
}

// Name returns the unique tool identifier.
func (b *BaseTool) Name() string { return b.ToolName }

// Description returns a concise summary of tool capabilities.
func (b *BaseTool) Description() string { return b.ToolDesc }

// InputSchema returns the JSON schema defining input arguments.
func (b *BaseTool) InputSchema() contracts.JSONSchema { return b.Schema }

// Strict returns whether structured output is enforced.
func (b *BaseTool) Strict() bool { return b.IsStrict }

// Execute runs the underlying tool logic.
func (b *BaseTool) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	if b.ExecHandler == nil {
		return contracts.ToolResult{Output: "tool has no execution handler", IsError: true}, nil
	}
	return b.ExecHandler(ctx, input)
}

// StringArg extracts a named string argument from input map, checking multiple candidate keys.
func StringArg(input any, keys ...string) string {
	m, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s)
			}
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

// IntArg extracts a named integer argument from input map, checking multiple candidate keys.
func IntArg(input any, defaultVal int, keys ...string) int {
	m, ok := input.(map[string]any)
	if !ok {
		return defaultVal
	}
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			switch v := val.(type) {
			case int:
				return v
			case int64:
				return int(v)
			case float64:
				return int(v)
			case string:
				var parsed int
				if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &parsed); err == nil {
					return parsed
				}
			}
		}
	}
	return defaultVal
}

// BoolArg extracts a named boolean argument from input map, checking multiple candidate keys.
func BoolArg(input any, defaultVal bool, keys ...string) bool {
	m, ok := input.(map[string]any)
	if !ok {
		return defaultVal
	}
	for _, k := range keys {
		if val, exists := m[k]; exists && val != nil {
			switch v := val.(type) {
			case bool:
				return v
			case string:
				s := strings.ToLower(strings.TrimSpace(v))
				if s == "true" || s == "1" || s == "yes" {
					return true
				}
				if s == "false" || s == "0" || s == "no" {
					return false
				}
			}
		}
	}
	return defaultVal
}

// NormalizePath normalizes a repository-relative path.
func NormalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	cleaned := filepath.Clean(p)
	if cleaned == "." {
		return ""
	}
	return strings.ReplaceAll(cleaned, "\\", "/")
}

// AddLineNumbers adds 1-based or offset line numbers to text content.
func AddLineNumbers(content string, baseLineNumber int) string {
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(fmt.Sprintf("%d: %s", baseLineNumber+i, line))
	}
	return sb.String()
}

// ShellQuote safely quotes an argument for bash execution.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// TruncateWithNotice truncates string content to maxLen with a notice.
func TruncateWithNotice(s string, maxLen int, notice string) string {
	if len(s) <= maxLen {
		return s
	}
	head := s[:maxLen]
	lastNL := strings.LastIndex(head, "\n")
	if lastNL > maxLen/2 {
		head = head[:lastNL]
	}
	return head + "\n" + notice
}

// SimpleExecutionMetrics tracks execution counts and latencies per tool.
type ExecutionMetrics struct {
	mu           sync.Mutex
	CallCounts   map[string]int
	ErrorCounts  map[string]int
	TotalLatency map[string]time.Duration
}

// NewExecutionMetrics constructs a thread-safe ExecutionMetrics tracker.
func NewExecutionMetrics() *ExecutionMetrics {
	return &ExecutionMetrics{
		CallCounts:   make(map[string]int),
		ErrorCounts:  make(map[string]int),
		TotalLatency: make(map[string]time.Duration),
	}
}

// Record records a tool execution.
func (m *ExecutionMetrics) Record(toolName string, duration time.Duration, isError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallCounts[toolName]++
	if isError {
		m.ErrorCounts[toolName]++
	}
	m.TotalLatency[toolName] += duration
}

// Snapshot returns a copy of current metrics.
func (m *ExecutionMetrics) Snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]any)
	for k, calls := range m.CallCounts {
		errs := m.ErrorCounts[k]
		dur := m.TotalLatency[k]
		avg := time.Duration(0)
		if calls > 0 {
			avg = dur / time.Duration(calls)
		}
		res[k] = map[string]any{
			"calls":        calls,
			"errors":       errs,
			"total_ms":     dur.Milliseconds(),
			"avg_ms":       avg.Milliseconds(),
		}
	}
	return res
}
