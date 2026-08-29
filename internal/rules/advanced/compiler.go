package advanced

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CompiledDetector represents an executable in-memory security matcher.
type CompiledDetector struct {
	Rule        EnterpriseRuleDefinition
	Matcher     *regexp.Regexp
	CompiledAt  time.Time
}

// RuleCompiler turns declarative rule patterns into high-speed regex matchers with ReDoS protections.
type RuleCompiler struct{}

func NewRuleCompiler() *RuleCompiler {
	return &RuleCompiler{}
}

// CompileRule validates and compiles a pattern into an active detector.
func (c *RuleCompiler) CompileRule(def EnterpriseRuleDefinition) (*CompiledDetector, error) {
	pattern := strings.TrimSpace(def.PatternRaw)
	if pattern == "" {
		return nil, fmt.Errorf("rule %s has empty pattern", def.RuleKey)
	}

	// ReDoS / Length safety guard
	if len(pattern) > 1000 {
		return nil, fmt.Errorf("rule pattern exceeds safe length limit (1000 chars)")
	}

	matcher, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed compiling regex for rule %s: %w", def.RuleKey, err)
	}

	return &CompiledDetector{
		Rule:       def,
		Matcher:    matcher,
		CompiledAt: time.Now().UTC(),
	}, nil
}

// ScanText scans input code against all active compiled detectors.
func ScanText(detectors []*CompiledDetector, filename, content string) []EnterpriseFinding {
	var findings []EnterpriseFinding

	lines := strings.Split(content, "\n")
	for lineIdx, line := range lines {
		for _, d := range detectors {
			if d.Matcher.MatchString(line) {
				findings = append(findings, EnterpriseFinding{
					ID:          uuid.New(),
					RuleKey:     d.Rule.RuleKey,
					Title:       d.Rule.Title,
					CWE:         d.Rule.CWE,
					OWASPCat:    d.Rule.OWASPCat,
					Severity:    d.Rule.Severity,
					FilePath:    filename,
					LineNumber:  lineIdx + 1,
					CodeSnippet: strings.TrimSpace(line),
					Remediation: d.Rule.Remediation,
				})
			}
		}
	}

	return findings
}

// EnterpriseFinding describes a concrete defect detected in code.
type EnterpriseFinding struct {
	ID          uuid.UUID `json:"id"`
	RuleKey     string    `json:"rule_key"`
	Title       string    `json:"title"`
	CWE         string    `json:"cwe"`
	OWASPCat    string    `json:"owasp_cat"`
	Severity    string    `json:"severity"`
	FilePath    string    `json:"file_path"`
	LineNumber  int       `json:"line_number"`
	CodeSnippet string    `json:"code_snippet"`
	Remediation string    `json:"remediation"`
}
