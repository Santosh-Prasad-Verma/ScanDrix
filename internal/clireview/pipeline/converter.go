package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// CliInputConverter transforms CLI requests, diffs, and pipeline results.
type CliInputConverter struct{}

// NewCliInputConverter instantiates a new converter.
func NewCliInputConverter() *CliInputConverter {
	return &CliInputConverter{}
}

// ConvertToFileChanges converts CLI input into FileChange items.
func (c *CliInputConverter) ConvertToFileChanges(input domain.CliReviewInput) []domain.FileChange {
	if input.Config != nil && len(input.Config.Files) > 0 {
		return c.ConvertFromFilesList(input.Config.Files)
	}
	return c.ParseFilesFromDiff(input.Diff)
}

var diffGitRegex = regexp.MustCompile(`(?m)^diff --git\s+`)

// ParseFilesFromDiff parses a unified diff string into FileChange records.
func (c *CliInputConverter) ParseFilesFromDiff(unifiedDiff string) []domain.FileChange {
	if strings.TrimSpace(unifiedDiff) == "" {
		return nil
	}

	cleaned := c.RemoveContextSections(unifiedDiff)
	indices := diffGitRegex.FindAllStringIndex(cleaned, -1)
	if len(indices) == 0 {
		return nil
	}

	var blocks []string
	for i := 0; i < len(indices); i++ {
		start := indices[i][0]
		var end int
		if i+1 < len(indices) {
			end = indices[i+1][0]
		} else {
			end = len(cleaned)
		}
		blocks = append(blocks, cleaned[start:end])
	}

	var files []domain.FileChange
	for _, block := range blocks {
		filename := c.ExtractFilename(block)
		if filename == "" {
			continue
		}

		adds, dels := c.CountChanges(block)
		status := c.DetectFileStatus(block)

		files = append(files, domain.FileChange{
			Filename:          filename,
			Patch:             block,
			PatchWithLinesStr: c.AddLineNumbersToPatch(block, filename),
			Status:            status,
			Additions:         adds,
			Deletions:         dels,
			Changes:           adds + dels,
			SHA:               c.GenerateCliSHA(filename),
		})
	}

	return files
}

// ConvertFromFilesList converts explicitly passed files into FileChanges.
func (c *CliInputConverter) ConvertFromFilesList(files []domain.CliFileInput) []domain.FileChange {
	var out []domain.FileChange
	for _, f := range files {
		adds, dels := c.CountChanges(f.Diff)
		status := f.Status
		if status == "" {
			status = "modified"
		}
		out = append(out, domain.FileChange{
			Filename:          f.Path,
			Patch:             f.Diff,
			PatchWithLinesStr: c.AddLineNumbersToPatch(f.Diff, f.Path),
			Status:            status,
			Additions:         adds,
			Deletions:         dels,
			Changes:           adds + dels,
			SHA:               c.GenerateCliSHA(f.Path),
			Content:           f.Content,
		})
	}
	return out
}

// ConvertToCliResponse formats findings into the standard CliReviewResponse.
func (c *CliInputConverter) ConvertToCliResponse(suggestions []domain.CodeSuggestion, filesCount int, startTime time.Time) domain.CliReviewResponse {
	duration := time.Since(startTime).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	var issues []domain.CliReviewIssue
	for _, s := range suggestions {
		issues = append(issues, c.ConvertToCliIssue(s))
	}

	summary := fmt.Sprintf("Found %d issue(s) across %d file(s)", len(issues), filesCount)
	if len(issues) == 0 {
		summary = fmt.Sprintf("No issues found across %d file(s)", filesCount)
	}

	return domain.CliReviewResponse{
		Summary:       summary,
		Issues:        issues,
		FilesAnalyzed: filesCount,
		Duration:      duration,
	}
}

// ConvertToCliIssue converts a pipeline suggestion into a CliReviewIssue.
func (c *CliInputConverter) ConvertToCliIssue(suggestion domain.CodeSuggestion) domain.CliReviewIssue {
	severity := strings.ToLower(suggestion.Severity)
	if severity == "" {
		severity = "medium"
	}

	issue := domain.CliReviewIssue{
		File:           suggestion.File,
		Line:           suggestion.Line,
		EndLine:        suggestion.EndLine,
		Severity:       severity,
		Category:       suggestion.Category,
		Message:        suggestion.Message,
		Suggestion:     suggestion.Suggestion,
		Recommendation: suggestion.Recommendation,
		RuleID:         suggestion.RuleID,
		Fixable:        suggestion.Fixable,
	}

	if suggestion.Fixable && suggestion.Replacement != "" {
		start := 0
		if suggestion.StartIndex != nil {
			start = *suggestion.StartIndex
		}
		end := start + len(suggestion.Replacement)
		if suggestion.EndIndex != nil {
			end = *suggestion.EndIndex
		}
		issue.Fix = &domain.CliReviewIssueFix{
			Replacement: suggestion.Replacement,
		}
		issue.Fix.Range.Start = start
		issue.Fix.Range.End = end
	}

	return issue
}

// RemoveContextSections removes binary diff markers and commit headers before parsing.
func (c *CliInputConverter) RemoveContextSections(unifiedDiff string) string {
	lines := strings.Split(unifiedDiff, "\n")
	var cleaned []string
	skipping := false

	for _, line := range lines {
		if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
			skipping = true
			continue
		}
		if skipping && strings.HasPrefix(line, "diff --git") {
			skipping = false
		}
		if !skipping {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

var (
	diffHeaderRegex = regexp.MustCompile(`(?m)^diff --git a/(.+?) b/(.+?)$`)
	plusPlusRegex   = regexp.MustCompile(`(?m)^\+\+\+ b/(.+?)$`)
)

// ExtractFilename determines the modified filename from a diff header.
func (c *CliInputConverter) ExtractFilename(diffBlock string) string {
	if m := plusPlusRegex.FindStringSubmatch(diffBlock); len(m) > 1 && m[1] != "/dev/null" {
		return strings.TrimSpace(m[1])
	}
	if m := diffHeaderRegex.FindStringSubmatch(diffBlock); len(m) > 2 {
		if m[2] != "/dev/null" {
			return strings.TrimSpace(m[2])
		}
		return strings.TrimSpace(m[1])
	}
	return ""
}

// CountChanges sums added and deleted lines in a patch hunk.
func (c *CliInputConverter) CountChanges(diffBlock string) (int, int) {
	adds := 0
	dels := 0
	for _, line := range strings.Split(diffBlock, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			adds++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			dels++
		}
	}
	return adds, dels
}

// DetectFileStatus infers status from git diff metadata headers.
func (c *CliInputConverter) DetectFileStatus(diffBlock string) string {
	if strings.Contains(diffBlock, "new file mode") {
		return "added"
	}
	if strings.Contains(diffBlock, "deleted file mode") {
		return "deleted"
	}
	if strings.Contains(diffBlock, "similarity index") || strings.Contains(diffBlock, "rename from") {
		return "renamed"
	}
	return "modified"
}

// GenerateCliSHA hashes the filename with salt to produce a stable dummy commit/file SHA.
func (c *CliInputConverter) GenerateCliSHA(filename string) string {
	h := sha256.Sum256([]byte("scandrix:cli:" + filename))
	return hex.EncodeToString(h[:20])
}

// AddLineNumbersToPatch formats patch lines with line numbers for prompt synthesis.
func (c *CliInputConverter) AddLineNumbersToPatch(patch, filename string) string {
	lines := strings.Split(patch, "\n")
	var out []string
	curLine := 0

	for _, l := range lines {
		if strings.HasPrefix(l, "@@") {
			var newStart int
			n, _ := fmt.Sscanf(l, "@@ -%*d,%*d +%d,%*d @@", &newStart)
			if n == 1 {
				curLine = newStart
			} else {
				curLine = 1
			}
			out = append(out, l)
			continue
		}

		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			out = append(out, fmt.Sprintf("%4d + %s", curLine, l[1:]))
			curLine++
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			out = append(out, fmt.Sprintf("     - %s", l[1:]))
		} else if strings.HasPrefix(l, " ") {
			out = append(out, fmt.Sprintf("%4d   %s", curLine, l[1:]))
			curLine++
		} else {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
