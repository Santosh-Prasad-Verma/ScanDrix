package rules

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// ═══════════════════════════════════════════════════════════════
// 1. RULE SPECIFICATIONS & STRATEGIES (Regex patterns & rule metadata)
// ═══════════════════════════════════════════════════════════════

// RuleType defines the evaluation strategy.
type RuleType string

const (
	RuleTypePatternMatch   RuleType = "PATTERN_MATCH"
	RuleTypeProhibitedCode RuleType = "PROHIBITED_CODE"
)

// RuleSpec defines a user or organization custom review rule.
// RuleSpec is a single compiled rule definition.
//
// The JSON tags match dtos.RuleResponse, which is what the workspace rule
// endpoints return. Without them this type serialised with its Go field names
// (ID, Name, PathPattern), so GET /rules/catalog answered in a different casing
// from GET /rules and every client had to accept both spellings.
type RuleSpec struct {
	ID          uuid.UUID              `json:"id"`
	Name        string                 `json:"name"`
	PathPattern string                 `json:"path_pattern"` // Glob matching file paths, e.g. "*.go", "api/**"
	RegexRule   string                 `json:"regex_rule"`   // Regular expression searched across added lines
	Severity    models.FindingSeverity `json:"severity"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Remediation string                 `json:"remediation"`
}

// ═══════════════════════════════════════════════════════════════
// 2. COMPILED RULE REGISTRY (Precompiled regex map for high performance)
// ═══════════════════════════════════════════════════════════════

// Evaluator checks git diff patches against active workspace policy rules.
type Evaluator struct {
	compiledRules map[uuid.UUID]*compiledRule
}

type compiledRule struct {
	spec  RuleSpec
	regex *regexp.Regexp
}

// ═══════════════════════════════════════════════════════════════
// 3. EVALUATOR COMPILATION (Rule validation & regex compilation)
// ═══════════════════════════════════════════════════════════════

// NewEvaluator compiles and validates custom policy rules.
func NewEvaluator(specs []RuleSpec) (*Evaluator, error) {
	compiled := make(map[uuid.UUID]*compiledRule)
	for _, spec := range specs {
		rx, err := regexp.Compile(spec.RegexRule)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in rule '%s': %w", spec.Name, err)
		}
		compiled[spec.ID] = &compiledRule{
			spec:  spec,
			regex: rx,
		}
	}
	return &Evaluator{compiledRules: compiled}, nil
}

// ═══════════════════════════════════════════════════════════════
// 4. DIFF PATCH RULE ENGINE (Diff hunks traversal & finding fingerprinting)
// ═══════════════════════════════════════════════════════════════

// EvaluatePatches runs all active rules against changed hunks in a pull request.
func (e *Evaluator) EvaluatePatches(reviewID, workspaceID uuid.UUID, patches []*diff.FilePatch) []models.CodeFinding {
	var findings []models.CodeFinding

	for _, patch := range patches {
		if patch.IsBinary || patch.IsDeleted {
			continue
		}

		targetPath := patch.NewPath
		for _, cr := range e.compiledRules {
			// Check if file path matches rule glob pattern
			if cr.spec.PathPattern != "" {
				matched, err := filepath.Match(cr.spec.PathPattern, filepath.Base(targetPath))
				if err != nil || !matched {
					continue
				}
			}

			// Scan only added lines in diff hunks
			for _, hunk := range patch.Hunks {
				for _, line := range hunk.Lines {
					if line.Type != diff.LineAddition {
						continue
					}

					trimmed := strings.TrimSpace(line.Content)
					// Skip comments and docstrings
					if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "<!--") {
						continue
					}

					// Skip rule definitions, prompt strings, and log message literals
					if strings.Contains(line.Content, "Prompt:") || strings.Contains(line.Content, "Description:") || strings.Contains(line.Content, "Remediation:") || strings.Contains(line.Content, "CodeFinding{") || strings.Contains(line.Content, "RegexRule:") {
						continue
					}

					if cr.regex.MatchString(line.Content) {
						fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", cr.spec.Name, targetPath, line.NewLineNo))))
						findings = append(findings, models.CodeFinding{
							ID:          uuid.New(),
							ReviewID:    reviewID,
							WorkspaceID: workspaceID,
							FilePath:    targetPath,
							StartLine:   line.NewLineNo,
							EndLine:     line.NewLineNo,
							Severity:    cr.spec.Severity,
							Category:    cr.spec.Category,
							Title:       cr.spec.Name,
							Description: cr.spec.Description,
							Remediation: cr.spec.Remediation,
							Fingerprint: fingerprint,
						})
					}
				}
			}
		}
	}

	return findings
}
