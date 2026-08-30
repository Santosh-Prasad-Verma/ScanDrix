package parser

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
)

// Standard rule file detection patterns matching Kodus & industry conventions
var RuleFilePatterns = []string{
	".kody/rules/**/*.md",
	".scandrix/rules/**/*.md",
	"rules/**/*.md",
	"AGENTS.md",
	".agents.md",
	".agent.md",
	"CLAUDE.md",
	".cursorrules",
	".cursor/rules/**/*.md",
}

// MarkdownRuleDefinition represents a parsed rule from a markdown or config file.
type MarkdownRuleDefinition struct {
	ID           uuid.UUID              `json:"id"`
	Name         string                 `json:"name"`
	Severity     models.FindingSeverity `json:"severity"`
	Category     string                 `json:"category"`
	PathPatterns []string               `json:"path_patterns"`
	Description  string                 `json:"description"`
	Remediation  string                 `json:"remediation"`
	GoodExamples []string               `json:"good_examples,omitempty"`
	BadExamples  []string               `json:"bad_examples,omitempty"`
	RawContent   string                 `json:"raw_content"`
	SourcePath   string                 `json:"source_path"`
}

// ParseMarkdownRule extracts frontmatter and body sections from a markdown rule file.
func ParseMarkdownRule(sourcePath string, content []byte) (*MarkdownRuleDefinition, error) {
	rule := &MarkdownRuleDefinition{
		ID:           uuid.New(),
		Name:         extractDefaultName(sourcePath),
		Severity:     models.SeverityMedium,
		Category:     "SECURITY_BEST_PRACTICE",
		PathPatterns: []string{"*"},
		SourcePath:   sourcePath,
		RawContent:   string(content),
		GoodExamples: make([]string, 0),
		BadExamples:  make([]string, 0),
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	var inFrontmatter bool
	var frontmatterLines []string
	var bodyLines []string

	lineIdx := 0
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if lineIdx == 0 && trimmed == "---" {
			inFrontmatter = true
			lineIdx++
			continue
		}

		if inFrontmatter {
			if trimmed == "---" {
				inFrontmatter = false
				lineIdx++
				continue
			}
			frontmatterLines = append(frontmatterLines, line)
		} else {
			bodyLines = append(bodyLines, line)
		}
		lineIdx++
	}

	// Parse YAML frontmatter attributes
	parseFrontmatter(frontmatterLines, rule)

	// Parse body for title, descriptions, and examples
	parseBody(bodyLines, rule)

	return rule, nil
}

func extractDefaultName(sourcePath string) string {
	base := filepath.Base(sourcePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")

	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

func parseFrontmatter(lines []string, rule *MarkdownRuleDefinition) {
	for _, line := range lines {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch key {
		case "title", "name":
			if val != "" {
				rule.Name = val
			}
		case "severity":
			switch strings.ToUpper(val) {
			case "CRITICAL":
				rule.Severity = models.SeverityCritical
			case "HIGH":
				rule.Severity = models.SeverityHigh
			case "LOW":
				rule.Severity = models.SeverityLow
			case "INFO":
				rule.Severity = models.SeverityInfo
			default:
				rule.Severity = models.SeverityMedium
			}
		case "category":
			if val != "" {
				rule.Category = val
			}
		case "paths", "path", "globs", "include":
			rule.PathPatterns = SplitRulePathGlobs(val)
		case "description":
			if val != "" {
				rule.Description = val
			}
		}
	}
}

func parseBody(lines []string, rule *MarkdownRuleDefinition) {
	var descBuilder strings.Builder
	var currentSection string
	var sectionContent strings.Builder

	flushSection := func() {
		content := strings.TrimSpace(sectionContent.String())
		if content == "" {
			return
		}
		switch currentSection {
		case "good":
			rule.GoodExamples = append(rule.GoodExamples, content)
		case "bad":
			rule.BadExamples = append(rule.BadExamples, content)
		case "remediation":
			rule.Remediation = content
		}
		sectionContent.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(trimmed, "# ") && rule.Name == "" {
			rule.Name = strings.TrimSpace(trimmed[2:])
			continue
		}

		if strings.HasPrefix(lower, "## good") || strings.HasPrefix(lower, "### good") {
			flushSection()
			currentSection = "good"
			continue
		} else if strings.HasPrefix(lower, "## bad") || strings.HasPrefix(lower, "### bad") {
			flushSection()
			currentSection = "bad"
			continue
		} else if strings.HasPrefix(lower, "## remediation") || strings.HasPrefix(lower, "## fix") {
			flushSection()
			currentSection = "remediation"
			continue
		}

		if currentSection != "" {
			sectionContent.WriteString(line + "\n")
		} else {
			descBuilder.WriteString(line + "\n")
		}
	}
	flushSection()

	if rule.Description == "" {
		rule.Description = strings.TrimSpace(descBuilder.String())
	}
}

// SplitRulePathGlobs splits string containing multiple globs (comma, space, or bracket delimited).
func SplitRulePathGlobs(raw string) []string {
	raw = strings.Trim(raw, "[]")
	if raw == "" {
		return []string{"*"}
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})

	var res []string
	for _, p := range parts {
		cleaned := strings.TrimSpace(p)
		cleaned = strings.Trim(cleaned, `"'`)
		if cleaned != "" {
			res = append(res, cleaned)
		}
	}

	if len(res) == 0 {
		return []string{"*"}
	}
	return res
}

// MatchesPath checks whether a file path matches any of the rule's path globs.
func MatchesPath(globs []string, filePath string) bool {
	normFile := filepath.ToSlash(strings.ToLower(filePath))

	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g == "*" || g == "**" || g == "**/*" {
			return true
		}

		normGlob := filepath.ToSlash(strings.ToLower(g))

		// Handle directory prefix e.g. "apps/api/**"
		if strings.HasSuffix(normGlob, "/**") {
			prefix := strings.TrimSuffix(normGlob, "/**")
			if strings.HasPrefix(normFile, prefix+"/") || normFile == prefix {
				return true
			}
		}

		matched, err := filepath.Match(normGlob, normFile)
		if err == nil && matched {
			return true
		}

		// Regex fallback for complex globs
		regexPattern := "^" + regexp.QuoteMeta(normGlob)
		regexPattern = strings.ReplaceAll(regexPattern, `\*\*`, `.*`)
		regexPattern = strings.ReplaceAll(regexPattern, `\*`, `[^/]*`) + "$"
		if re, err := regexp.Compile(regexPattern); err == nil {
			if re.MatchString(normFile) {
				return true
			}
		}
	}
	return false
}

// ConvertToRuleSpec translates MarkdownRuleDefinition into an engine RuleSpec.
func (m *MarkdownRuleDefinition) ConvertToRuleSpec() rules.RuleSpec {
	pathPattern := "*"
	if len(m.PathPatterns) > 0 {
		pathPattern = m.PathPatterns[0]
	}

	return rules.RuleSpec{
		ID:          m.ID,
		Name:        m.Name,
		PathPattern: pathPattern,
		Severity:    m.Severity,
		Category:    m.Category,
		Description: m.Description,
		Remediation: m.Remediation,
	}
}
