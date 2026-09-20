// Package rules provides rule file parsing, markdown frontmatter extraction, and pattern matching for ScanDrix.
package rules

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RuleExample holds a bad or good code snippet extracted from a rule file.
type RuleExample struct {
	Snippet   string `json:"snippet"`
	IsCorrect bool   `json:"isCorrect"`
}

// ParsedRuleFile represents the structured content of a parsed rule file.
type ParsedRuleFile struct {
	UUID         string        `json:"uuid,omitempty"`
	Title        string        `json:"title"`
	Description  string        `json:"description,omitempty"`
	Rule         string        `json:"rule"`
	Prompt       string        `json:"prompt,omitempty"`
	Path         string        `json:"path"`
	Severity     string        `json:"severity"`
	Scope        string        `json:"scope"`
	Enabled      bool          `json:"enabled"`
	FilePatterns []string      `json:"filePatterns,omitempty"`
	BadExamples  []string      `json:"badExamples,omitempty"`
	GoodExamples []string      `json:"goodExamples,omitempty"`
	Examples     []RuleExample `json:"examples"`
}

var frontmatterRegex = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)

type ruleFrontmatter struct {
	UUID        string `yaml:"uuid"`
	Title       string `yaml:"title"`
	SeverityMin string `yaml:"severity_min"`
	Severity    string `yaml:"severity"`
	Scope       string `yaml:"scope"`
	Path        any    `yaml:"path"`
	Enabled     *bool  `yaml:"enabled"`
}

func normalizeSeverity(val string) string {
	s := strings.ToLower(strings.TrimSpace(val))
	switch s {
	case "critical", "high", "medium", "low":
		return s
	default:
		return "medium"
	}
}

func normalizePath(val any) string {
	if val == nil {
		return "**/*"
	}
	switch v := val.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return "**/*"
		}
		return trimmed
	case []any:
		var globs []string
		for _, item := range v {
			if str, ok := item.(string); ok {
				trimmed := strings.TrimSpace(str)
				if trimmed != "" {
					globs = append(globs, trimmed)
				}
			}
		}
		if len(globs) > 0 {
			return strings.Join(globs, ",")
		}
	}
	return "**/*"
}

// ExtractExamplesFromBody extracts bad and good code examples from the markdown body.
func ExtractExamplesFromBody(body string) []RuleExample {
	var examples []RuleExample
	lines := strings.Split(body, "\n")

	var currentKind *bool
	var currentLevel *int
	inFence := false
	fenceMarker := ""
	var snippetLines []string

	flushSnippet := func() {
		if currentKind != nil && len(snippetLines) > 0 {
			examples = append(examples, RuleExample{
				Snippet:   strings.Join(snippetLines, "\n"),
				IsCorrect: *currentKind,
			})
		}
		snippetLines = nil
	}

	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	fenceRegex := regexp.MustCompile(`^\s*(` + "`{3,}" + `|~{3,})`)
	badWords := regexp.MustCompile(`(?i)\b(bad|incorrect|wrong)\b`)
	goodWords := regexp.MustCompile(`(?i)\b(good|correct)\b`)

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmedLine, fenceMarker) {
				inFence = false
				flushSnippet()
			} else {
				snippetLines = append(snippetLines, line)
			}
			continue
		}

		if match := headingRegex.FindStringSubmatch(line); len(match) > 2 {
			level := len(match[1])
			text := match[2]
			if badWords.MatchString(text) {
				f := false
				currentKind = &f
				currentLevel = &level
			} else if goodWords.MatchString(text) {
				t := true
				currentKind = &t
				currentLevel = &level
			} else if currentLevel != nil && level <= *currentLevel {
				currentKind = nil
				currentLevel = nil
			}
			continue
		}

		if fenceMatch := fenceRegex.FindStringSubmatch(line); len(fenceMatch) > 1 && currentKind != nil {
			inFence = true
			fenceMarker = fenceMatch[1]
			snippetLines = nil
		}
	}

	if inFence {
		flushSnippet()
	}

	return examples
}

// ParseRuleFile parses a markdown file containing YAML frontmatter and rule body into ParsedRuleFile.
func ParseRuleFile(content string) *ParsedRuleFile {
	content = strings.TrimPrefix(content, "\ufeff")
	if content == "" {
		return nil
	}

	loc := frontmatterRegex.FindStringSubmatchIndex(content)
	if loc == nil || len(loc) < 4 {
		// Fallback: try parsing direct YAML
		var rawMap map[string]any
		if err := yaml.Unmarshal([]byte(content), &rawMap); err == nil && len(rawMap) > 0 {
			titleVal, _ := rawMap["title"].(string)
			titleVal = strings.TrimSpace(titleVal)
			if titleVal != "" {
				ruleBody := ""
				if r, ok := rawMap["rule"].(string); ok {
					ruleBody = strings.TrimSpace(r)
				} else if p, ok := rawMap["prompt"].(string); ok {
					ruleBody = strings.TrimSpace(p)
				} else if d, ok := rawMap["description"].(string); ok {
					ruleBody = strings.TrimSpace(d)
				}
				if ruleBody == "" {
					ruleBody = titleVal
				}

				severityVal := "medium"
				if s, ok := rawMap["severity_min"].(string); ok && s != "" {
					severityVal = normalizeSeverity(s)
				} else if s, ok := rawMap["severity"].(string); ok && s != "" {
					severityVal = normalizeSeverity(s)
				}

				scopeVal := "file"
				if sc, ok := rawMap["scope"].(string); ok && strings.EqualFold(sc, "pull-request") {
					scopeVal = "pull-request"
				}

				enabledVal := true
				if en, ok := rawMap["enabled"].(bool); ok {
					enabledVal = en
				}

				uuidVal := ""
				if u, ok := rawMap["uuid"].(string); ok {
					uuidVal = strings.TrimSpace(u)
				}

				return &ParsedRuleFile{
					UUID:     uuidVal,
					Title:    titleVal,
					Rule:     ruleBody,
					Path:     normalizePath(rawMap["path"]),
					Severity: severityVal,
					Scope:    scopeVal,
					Enabled:  enabledVal,
					Examples: ExtractExamplesFromBody(ruleBody),
				}
			}
		}
		return nil
	}

	yamlContent := content[loc[2]:loc[3]]
	var fm ruleFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return nil
	}

	title := strings.TrimSpace(fm.Title)
	if title == "" {
		return nil
	}

	body := strings.TrimSpace(content[loc[1]:])
	if body == "" {
		return nil
	}

	severity := fm.SeverityMin
	if severity == "" {
		severity = fm.Severity
	}
	severity = normalizeSeverity(severity)

	scope := "file"
	if strings.EqualFold(fm.Scope, "pull-request") {
		scope = "pull-request"
	}

	enabled := true
	if fm.Enabled != nil {
		enabled = *fm.Enabled
	}

	return &ParsedRuleFile{
		UUID:      strings.TrimSpace(fm.UUID),
		Title:     title,
		Rule:      body,
		Path:      normalizePath(fm.Path),
		Severity:  severity,
		Scope:     scope,
		Enabled:   enabled,
		Examples:  ExtractExamplesFromBody(body),
	}
}

// IsRuleTemplateFile checks if filePath is under .scandrix/rules/ or matches rules/**/*.md.
func IsRuleTemplateFile(filePath string) bool {
	if filePath == "" {
		return false
	}
	clean := strings.ToLower(strings.ReplaceAll(filePath, "\\", "/"))

	if strings.HasPrefix(clean, ".scandrix/rules/") || strings.Contains(clean, "/.scandrix/rules/") {
		return true
	}
	if (strings.HasPrefix(clean, "rules/") || strings.Contains(clean, "/rules/")) && strings.HasSuffix(clean, ".md") {
		return true
	}
	return false
}
