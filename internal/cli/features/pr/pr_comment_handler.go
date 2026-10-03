// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package pr

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/scandrix/backend/internal/pathguard"
)

// ReviewCommentPayload defines the request body for posting an inline PR comment.
type ReviewCommentPayload struct {
	PRNumber int    `json:"prNumber"`
	RepoID   string `json:"repoId,omitempty"`
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Side     string `json:"side,omitempty"` // "RIGHT" or "LEFT"
	Body     string `json:"body"`
}

// PRCommentHandler manages posting, updating, and applying suggestions for pull requests.
type PRCommentHandler struct {
	apiClient *api.Client
}

// NewPRCommentHandler creates a new handler instance.
func NewPRCommentHandler(apiClient *api.Client) *PRCommentHandler {
	if apiClient == nil {
		apiClient = api.NewClient("", "", "")
	}
	return &PRCommentHandler{
		apiClient: apiClient,
	}
}

// FormatSuggestionMarkdown formats an issue suggestion as a GitHub/GitLab markdown suggestion block.
func FormatSuggestionMarkdown(ruleID, message, originalCode, replacementCode string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**ScanDrix [%s]**: %s\n\n", ruleID, message))
	if replacementCode != "" {
		sb.WriteString("```suggestion\n")
		sb.WriteString(replacementCode)
		if !strings.HasSuffix(replacementCode, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}
	return sb.String()
}

// PostReviewComment submits a review comment to the ScanDrix API for forwarding to SCM.
func (h *PRCommentHandler) PostReviewComment(ctx context.Context, payload ReviewCommentPayload) error {
	if payload.PRNumber <= 0 {
		return fmt.Errorf("invalid PR number: %d", payload.PRNumber)
	}
	if payload.FilePath == "" {
		return fmt.Errorf("filePath is required")
	}
	if strings.TrimSpace(payload.Body) == "" {
		return fmt.Errorf("comment body cannot be empty")
	}

	var resp map[string]any
	err := h.apiClient.Do(ctx, "POST", "/v1/scm/comments", payload, &resp)
	if err != nil {
		return fmt.Errorf("failed to post review comment: %w", err)
	}
	return nil
}

// ApplyLocalSuggestion writes the replacement code directly to the local target file.
// ApplyLocalSuggestion rewrites a region of a file in place. filePath comes from
// a review suggestion, so root bounds it: without a boundary the suggestion
// chooses which file on the host gets rewritten.
func ApplyLocalSuggestion(root, filePath string, startLine, endLine int, replacement string) error {
	resolved, pathErr := pathguard.ResolvePath(root, filePath)
	if pathErr != nil {
		return fmt.Errorf("refusing to modify %q: %w", filePath, pathErr)
	}
	filePath = resolved

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read target file %s: %w", filePath, err)
	}

	lines := strings.Split(string(data), "\n")
	if startLine < 1 || startLine > len(lines) {
		return fmt.Errorf("start line %d out of bounds (1..%d)", startLine, len(lines))
	}
	if endLine < startLine || endLine > len(lines) {
		endLine = startLine
	}

	newLines := make([]string, 0, len(lines))
	newLines = append(newLines, lines[:startLine-1]...)
	newLines = append(newLines, strings.Split(replacement, "\n")...)
	if endLine < len(lines) {
		newLines = append(newLines, lines[endLine:]...)
	}

	out := strings.Join(newLines, "\n")
	if err := os.WriteFile(filePath, []byte(out), 0644); err != nil { // #nosec G703 -- filePath is re-resolved by pathguard.ResolvePath at the top of ApplyLocalSuggestion
		return fmt.Errorf("failed to write patched file %s: %w", filePath, err)
	}

	utils.Success("✔ Applied suggestion to %s (lines %d-%d)", filePath, startLine, endLine)
	return nil
}
