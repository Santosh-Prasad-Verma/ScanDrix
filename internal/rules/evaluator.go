package rules

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// RuleType defines the evaluation strategy.
type RuleType string

const (
	RuleTypePatternMatch   RuleType = "PATTERN_MATCH"
	RuleTypeProhibitedCode RuleType = "PROHIBITED_CODE"
)

// RuleSpec defines a user or organization custom review rule.
type RuleSpec struct {
	ID          uuid.UUID
	Name        string
	PathPattern string // Glob matching file paths, e.g. "*.go", "api/**"
	RegexRule   string // Regular expression searched across added lines
	Severity    models.FindingSeverity
	Category    string
	Description string
	Remediation string
}

// Evaluator checks git diff patches against active workspace policy rules.
type Evaluator struct {
	compiledRules map[uuid.UUID]*compiledRule
}

type compiledRule struct {
	spec  RuleSpec
	regex *regexp.Regexp
}

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
