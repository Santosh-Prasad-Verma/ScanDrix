package review

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// HunkAgentContext models the standard agent-context JSON payload consumed by hunk.
type HunkAgentContext struct {
	Version int                    `json:"version"` // 1
	Summary string                 `json:"summary,omitempty"`
	Files   []HunkAgentContextFile `json:"files"`
}

// HunkAgentContextFile holds file-level inline annotations.
type HunkAgentContextFile struct {
	Path        string                `json:"path"`
	Summary     string                `json:"summary,omitempty"`
	Annotations []HunkAgentAnnotation `json:"annotations"`
}

// HunkAgentAnnotation models a single inline finding anchored to new diff lines.
type HunkAgentAnnotation struct {
	NewRange  [2]int `json:"newRange"` // [startLine, endLine]
	Summary   string `json:"summary"`
	Rationale string `json:"rationale,omitempty"`
	Markup    string `json:"markup,omitempty"` // Experimental STML
}

var severityGlyphMap = map[string]string{
	"critical": "‼",
	"error":    "✖",
	"warning":  "⚠",
	"info":     "ℹ",
}

const headlineMaxLen = 140

// ConvertReviewToHunkAgentContext converts types.ReviewResult into a HunkAgentContext document.
func ConvertReviewToHunkAgentContext(result *types.ReviewResult) *HunkAgentContext {
	filesMap := make(map[string]*HunkAgentContextFile)

	if result == nil {
		return &HunkAgentContext{Version: 1}
	}

	for _, issue := range result.Issues {
		if issue.File == "" || issue.Line <= 0 {
			continue
		}

		annotation := toAgentAnnotation(issue)
		bucket, exists := filesMap[issue.File]
		if !exists {
			bucket = &HunkAgentContextFile{
				Path:        issue.File,
				Annotations: []HunkAgentAnnotation{},
			}
			filesMap[issue.File] = bucket
		}
		bucket.Annotations = append(bucket.Annotations, annotation)
	}

	var files []HunkAgentContextFile
	for _, file := range filesMap {
		count := len(file.Annotations)
		label := "findings"
		if count == 1 {
			label = "finding"
		}
		file.Summary = fmt.Sprintf("%d %s", count, label)
		sort.Slice(file.Annotations, func(i, j int) bool {
			if file.Annotations[i].NewRange[0] != file.Annotations[j].NewRange[0] {
				return file.Annotations[i].NewRange[0] < file.Annotations[j].NewRange[0]
			}
			return file.Annotations[i].NewRange[1] < file.Annotations[j].NewRange[1]
		})
		files = append(files, *file)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &HunkAgentContext{
		Version: 1,
		Summary: buildAgentContextSummary(result),
		Files:   files,
	}
}

func toAgentAnnotation(issue types.ReviewIssue) HunkAgentAnnotation {
	start := issue.Line
	end := issue.EndLine
	if end < start {
		end = start
	}

	source := issue.Message
	if source == "" {
		source = issue.Suggestion
	}
	if source == "" {
		source = "ScanDrix finding"
	}

	head, _ := splitFirstSentence(source)
	sev := strings.ToLower(string(issue.Severity))
	if sev == "" {
		sev = "info"
	}
	glyph := severityGlyphMap[sev]
	headline := capHeadline(head, headlineMaxLen)
	summary := headline
	if glyph != "" {
		summary = fmt.Sprintf("%s %s", glyph, headline)
	}

	attribution := buildAttribution(issue, sev)

	var parts []string
	parts = append(parts, withTrailingPeriod(source))
	if issue.Suggestion != "" && issue.Suggestion != issue.Message {
		parts = append(parts, fmt.Sprintf("Fix: %s", withTrailingPeriod(issue.Suggestion)))
	}
	parts = append(parts, fmt.Sprintf("— Drixy · %s", attribution))

	return HunkAgentAnnotation{
		NewRange:  [2]int{start, end},
		Summary:   summary,
		Rationale: strings.Join(parts, " "),
		Markup:    buildSTMLMarkup(issue, sev, source, issue.Suggestion, attribution),
	}
}

func buildAttribution(issue types.ReviewIssue, severity string) string {
	bits := []string{fmt.Sprintf("severity %s", severity)}
	if issue.Category != "" {
		bits = append(bits, issue.Category)
	}
	if issue.RuleID != "" && !uuidPattern.MatchString(issue.RuleID) {
		bits = append(bits, issue.RuleID)
	}
	return strings.Join(bits, " · ")
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func buildSTMLMarkup(issue types.ReviewIssue, severity, body, advice, attribution string) string {
	color := "info"
	switch severity {
	case "critical", "error":
		color = "danger"
	case "warning":
		color = "warning"
	}

	cleanProse, links := extractMarkdownLinks(body)
	var blocks []string
	catBlock := ""
	if issue.Category != "" {
		catBlock = fmt.Sprintf(" <dim>%s</dim>", escapeSTML(issue.Category))
	}
	blocks = append(blocks, fmt.Sprintf(`<text><badge color="%s">%s</badge>%s</text>`, color, escapeSTML(severity), catBlock))
	blocks = append(blocks, fmt.Sprintf(`<p>%s</p>`, escapeSTML(cleanProse)))

	if advice != "" && advice != body {
		blocks = append(blocks, "<h3>Fix</h3>")
		blocks = append(blocks, fmt.Sprintf("<code>\n%s\n</code>", escapeSTML(advice)))
	}

	for _, l := range links {
		blocks = append(blocks, fmt.Sprintf("<p><dim>%s: %s</dim></p>", escapeSTML(l.Label), escapeSTML(l.URL)))
	}

	blocks = append(blocks, fmt.Sprintf(`<text><dim>— Drixy · %s</dim></text>`, escapeSTML(attribution)))
	return strings.Join(blocks, "\n")
}

// ExtractedLink represents a parsed markdown URL.
type ExtractedLink struct {
	Label string
	URL   string
}

var markdownLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)

func extractMarkdownLinks(text string) (string, []ExtractedLink) {
	var links []ExtractedLink
	cleaned := markdownLinkRegex.ReplaceAllStringFunc(text, func(match string) string {
		sub := markdownLinkRegex.FindStringSubmatch(match)
		if len(sub) == 3 {
			links = append(links, ExtractedLink{Label: sub[1], URL: sub[2]})
			return sub[1]
		}
		return match
	})
	return cleaned, links
}

func escapeSTML(text string) string {
	r := strings.ReplaceAll(text, "&", "&amp;")
	return strings.ReplaceAll(r, "<", "&lt;")
}

func withTrailingPeriod(text string) string {
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "!") || strings.HasSuffix(text, "?") {
		return text
	}
	return text + "."
}

func splitFirstSentence(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	for i := 0; i < len(trimmed)-2; i++ {
		if (trimmed[i] == '.' || trimmed[i] == '!' || trimmed[i] == '?') && trimmed[i+1] == ' ' {
			return strings.TrimSpace(trimmed[:i+1]), strings.TrimSpace(trimmed[i+2:])
		}
	}
	return trimmed, ""
}

func capHeadline(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	slice := text[:maxLen]
	lastSpace := strings.LastIndex(slice, " ")
	if lastSpace > maxLen*6/10 {
		slice = slice[:lastSpace]
	}
	return strings.TrimRight(slice, " ,.;:") + "…"
}

func buildAgentContextSummary(result *types.ReviewResult) string {
	if result == nil || len(result.Issues) == 0 {
		return "No issues identified by ScanDrix."
	}
	return fmt.Sprintf("ScanDrix detected %d issue(s) in review scope.", len(result.Issues))
}

// CountHunkAnnotations sums the total annotations across all files in the agent context.
func CountHunkAnnotations(hunkCtx *HunkAgentContext) int {
	if hunkCtx == nil {
		return 0
	}
	total := 0
	for _, f := range hunkCtx.Files {
		total += len(f.Annotations)
	}
	return total
}
