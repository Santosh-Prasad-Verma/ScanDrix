// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rule_file_parser.go
// ═══════════════════════════════════════════════════════════════

package utils

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParsedDrixyRuleFile represents a rule parsed verbatim from repository markdown files.
type ParsedDrixyRuleFile struct {
	UUID     string
	Title    string
	Rule     string
	Path     string
	Severity string
	Scope    string
	Enabled  bool
	Examples []ParsedDrixyRuleExample
}

// ParsedDrixyRuleExample captures good and bad code snippets from rule documentation.
type ParsedDrixyRuleExample struct {
	Snippet   string
	IsCorrect bool
}

var frontmatterRegex = regexp.MustCompile(`(?s)^---\r?\n([\s\S]*?)\r?\n---\r?\n?`)
var headingRegex = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
var fenceRegex = regexp.MustCompile(`^\s*(` + "`" + `{3,}|~{3,})`)

// ExtractExamplesFromBody scans the markdown body for Bad/Good example code snippets.
func ExtractExamplesFromBody(body string) []ParsedDrixyRuleExample {
	var examples []ParsedDrixyRuleExample
	lines := strings.Split(body, "\n")

	var currentKind *bool
	var currentLevel *int
	inFence := false
	fenceMarker := ""
	var snippetLines []string

	flushSnippet := func() {
		if currentKind != nil && len(snippetLines) > 0 {
			examples = append(examples, ParsedDrixyRuleExample{
				Snippet:   strings.Join(snippetLines, "\n"),
				IsCorrect: *currentKind,
			})
		}
		snippetLines = nil
	}

	for _, line := range lines {
		lineClean := strings.TrimRight(line, "\r")

		if inFence {
			if strings.HasPrefix(strings.TrimSpace(lineClean), fenceMarker) {
				inFence = false
				flushSnippet()
			} else {
				snippetLines = append(snippetLines, lineClean)
			}
			continue
		}

		if matches := headingRegex.FindStringSubmatch(lineClean); len(matches) == 3 {
			level := len(matches[1])
			text := strings.ToLower(matches[2])

			if strings.Contains(text, "bad") || strings.Contains(text, "incorrect") || strings.Contains(text, "wrong") {
				f := false
				currentKind = &f
				currentLevel = &level
			} else if strings.Contains(text, "good") || strings.Contains(text, "correct") {
				t := true
				currentKind = &t
				currentLevel = &level
			} else if currentLevel != nil && level <= *currentLevel {
				currentKind = nil
				currentLevel = nil
			}
			continue
		}

		if matches := fenceRegex.FindStringSubmatch(lineClean); len(matches) >= 2 && currentKind != nil {
			inFence = true
			fenceMarker = matches[1]
			snippetLines = nil
		}
	}

	if inFence {
		flushSnippet()
	}

	return examples
}

func ParseDrixyRuleFile(content string) *ParsedDrixyRuleFile {
	content = strings.TrimPrefix(content, "\ufeff")
	content = strings.TrimPrefix(content, "\xef\xbb\xbf")
	content = strings.TrimLeft(content, "\r\n\t ")
	if content == "" {
		return nil
	}

	match := frontmatterRegex.FindStringSubmatch(content)
	if len(match) < 2 {
		return nil
	}

	var frontmatter map[string]any
	if err := yaml.Unmarshal([]byte(match[1]), &frontmatter); err != nil || frontmatter == nil {
		return nil
	}

	title, _ := frontmatter["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}

	body := strings.TrimSpace(content[len(match[0]):])
	if body == "" {
		return nil
	}

	severity := "medium"
	if s, ok := frontmatter["severity_min"].(string); ok && s != "" {
		severity = strings.ToLower(strings.TrimSpace(s))
	} else if s, ok := frontmatter["severity"].(string); ok && s != "" {
		severity = strings.ToLower(strings.TrimSpace(s))
	}

	scope := "file"
	if sc, ok := frontmatter["scope"].(string); ok && sc == "pull-request" {
		scope = "pull-request"
	}

	pathPattern := "**/*"
	if p, ok := frontmatter["path"].(string); ok && strings.TrimSpace(p) != "" {
		pathPattern = strings.TrimSpace(p)
	} else if pSlice, ok := frontmatter["path"].([]any); ok && len(pSlice) > 0 {
		var parts []string
		for _, item := range pSlice {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				parts = append(parts, strings.TrimSpace(str))
			}
		}
		if len(parts) > 0 {
			pathPattern = strings.Join(parts, ",")
		}
	}

	uuidVal, _ := frontmatter["uuid"].(string)
	uuidVal = strings.TrimSpace(uuidVal)

	enabled := true
	if en, ok := frontmatter["enabled"].(bool); ok {
		enabled = en
	}

	return &ParsedDrixyRuleFile{
		UUID:     uuidVal,
		Title:    title,
		Rule:     body,
		Path:     pathPattern,
		Severity: severity,
		Scope:    scope,
		Enabled:  enabled,
		Examples: ExtractExamplesFromBody(body),
	}
}

// IsDrixyRuleTemplateFile verifies if a file is located in a structured rule template path.
func IsDrixyRuleTemplateFile(filePath string) bool {
	if filePath == "" {
		return false
	}
	lower := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))

	if strings.Contains(lower, ".drixy/rules/") || strings.HasPrefix(lower, ".drixy/rules/") {
		return true
	}
	if strings.HasSuffix(lower, ".md") && (strings.Contains(lower, "rules/") || strings.HasPrefix(lower, "rules/")) {
		return true
	}
	return false
}
