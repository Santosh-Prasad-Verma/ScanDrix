package review

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// -------------------------------------------------------------------------------------
// STML Markup Builder for Hunk TUI Integration
// -------------------------------------------------------------------------------------

var (
	mdLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	codeFenceRe = regexp.MustCompile("(?s)```(?:[a-zA-Z0-9_-]+)?\n?(.*?)\n?```")
)

// MarkdownLink represents an extracted markdown hyperlink [label](url).
type MarkdownLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}


// EscapeSTML escapes special markup characters for hunk's experimental STML format.
func EscapeSTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// ExtractMarkdownLinks splits markdown text into cleaned prose and extracted URLs.
func ExtractMarkdownLinks(input string) (string, []MarkdownLink) {
	var links []MarkdownLink
	matches := mdLinkRegex.FindAllStringSubmatch(input, -1)
	for _, m := range matches {
		if len(m) >= 3 {
			links = append(links, MarkdownLink{
				Label: strings.TrimSpace(m[1]),
				URL:   strings.TrimSpace(m[2]),
			})
		}
	}
	cleanProse := mdLinkRegex.ReplaceAllString(input, "$1")
	return cleanProse, links
}

// SplitAdvice separates natural language prose lead-ins from suggested code snippets.
func SplitAdvice(advice string) (lead string, code string) {
	advice = strings.TrimSpace(advice)
	if advice == "" {
		return "", ""
	}

	// Check for fenced code blocks first
	if matches := codeFenceRe.FindStringSubmatch(advice); len(matches) >= 2 {
		leadPart := strings.TrimSpace(codeFenceRe.ReplaceAllString(advice, ""))
		codePart := strings.TrimSpace(matches[1])
		return leadPart, codePart
	}

	// Check for backticked inline code
	if strings.Contains(advice, "`") {
		parts := strings.Split(advice, "`")
		if len(parts) >= 3 {
			leadPart := strings.TrimSpace(parts[0])
			codePart := strings.TrimSpace(parts[1])
			return leadPart, codePart
		}
	}

	return advice, ""
}

// WrapCodeBlock normalizes indented code lines for STML display.
func WrapCodeBlock(code string) string {
	lines := strings.Split(code, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Join(lines, "\n")
}

// BuildSTMLMarkup synthesizes rich STML annotations for hunk TUI rendering.
func BuildSTMLMarkup(
	issue types.ReviewIssue,
	severity string,
	body string,
	advice string,
	attribution string,
) string {
	color := "info"
	switch strings.ToLower(severity) {
	case "critical", "error":
		color = "danger"
	case "warning":
		color = "warning"
	}

	prose, links := ExtractMarkdownLinks(body)

	var blocks []string
	categoryBadge := ""
	if issue.Category != "" {
		categoryBadge = fmt.Sprintf(" <dim>%s</dim>", EscapeSTML(issue.Category))
	}
	blocks = append(blocks, fmt.Sprintf(`<text><badge color="%s">%s</badge>%s</text>`,
		color, EscapeSTML(strings.ToUpper(severity)), categoryBadge))
	blocks = append(blocks, fmt.Sprintf(`<p>%s</p>`, EscapeSTML(prose)))

	if advice != "" && advice != body {
		lead, code := SplitAdvice(advice)
		blocks = append(blocks, "<h3>Fix</h3>")
		if lead != "" {
			blocks = append(blocks, fmt.Sprintf(`<p>%s</p>`, EscapeSTML(lead)))
		}
		if code != "" {
			blocks = append(blocks, fmt.Sprintf("<code>\n%s\n</code>", EscapeSTML(WrapCodeBlock(code))))
		}
	}

	for _, link := range links {
		caption := ""
		if len(links) > 1 {
			caption = fmt.Sprintf("%s: ", EscapeSTML(strings.TrimRight(link.Label, ".: ")))
		}
		blocks = append(blocks, fmt.Sprintf(`<p><dim>%s%s</dim></p>`, caption, EscapeSTML(link.URL)))
	}

	blocks = append(blocks, fmt.Sprintf(`<text><dim>— Drixy · %s</dim></text>`, EscapeSTML(attribution)))

	return strings.Join(blocks, "\n")
}

