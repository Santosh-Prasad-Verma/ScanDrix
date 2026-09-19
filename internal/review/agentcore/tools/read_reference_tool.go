package tools

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// ReadReferenceToolOptions configures remote repository reference reading.
type ReadReferenceToolOptions struct {
	Fetcher SCMReferenceFetcher
	MaxLen  int
}

// NewReadReferenceTool creates the cross-repo file reading tool for referenced standards and guidelines.
func NewReadReferenceTool(opts ReadReferenceToolOptions) contracts.AgentTool {
	maxLen := opts.MaxLen
	if maxLen <= 0 {
		maxLen = MaxReadLength
	}

	return &BaseTool{
		ToolName: "readReference",
		ToolDesc: "Read a file from another repository or branch. Use this to fetch reference files mentioned in custom team rules " +
			"(e.g., coding standards, architecture patterns from other repos). Accepts full 'owner/repo' and optional branch.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"repo": {
					Type:        "string",
					Description: "Full repository name (e.g. 'my-org/shared-architecture' or 'my-org/design-system')",
				},
				"path": {
					Type:        "string",
					Description: "File path within the repository (e.g. 'docs/concurrency-standards.md')",
				},
				"branch": {
					Type:        "string",
					Description: "Branch or commit reference (default: 'main')",
				},
			},
			Required: []string{"repo", "path"},
		},
		IsStrict: false,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			repo := StringArg(input, "repo", "repository")
			path := NormalizePath(StringArg(input, "path", "file"))
			branch := StringArg(input, "branch", "ref")
			if branch == "" {
				branch = "main"
			}

			if repo == "" || path == "" {
				return contracts.ToolResult{Output: "Error: repo and path are required", IsError: true}, nil
			}

			if strings.Contains(path, "..") {
				return contracts.ToolResult{Output: "Error: path traversal with '..' is forbidden", IsError: true}, nil
			}

			if opts.Fetcher == nil {
				return contracts.ToolResult{
					Output: fmt.Sprintf("⚠️ SCM reference fetcher unavailable — cannot fetch %s from remote repo %s", path, repo),
				}, nil
			}

			content, err := opts.Fetcher.FetchFile(ctx.Context, repo, path, branch)
			if err != nil {
				return contracts.ToolResult{
					Output:  fmt.Sprintf("Error reading %s from %s#%s: %v", path, repo, branch, err),
					IsError: true,
				}, nil
			}

			if len(content) > maxLen {
				notice := fmt.Sprintf("... (truncated — %d characters total)", len(content))
				content = TruncateWithNotice(content, maxLen, notice)
			}

			return contracts.ToolResult{Output: content}, nil
		},
	}
}
