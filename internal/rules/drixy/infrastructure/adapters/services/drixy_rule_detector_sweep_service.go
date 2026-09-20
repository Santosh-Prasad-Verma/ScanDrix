// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rule_detector_sweep_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/pkg/models"
	"time"
)

// DetectorMatch captures a hit found by the T0 deterministic sweep.
type DetectorMatch struct {
	RuleUUID    string                 `json:"ruleUuid"`
	RuleTitle   string                 `json:"ruleTitle"`
	Severity    models.FindingSeverity `json:"severity"`
	FilePath    string                 `json:"filePath"`
	LineNumber  int                    `json:"lineNumber"`
	LineContent string                 `json:"lineContent"`
	Reason      string                 `json:"reason"`
}

// DrixyRuleDetectorSweepService evaluates code diffs against compiled T0 regex detectors.
type DrixyRuleDetectorSweepService struct{}

// NewDrixyRuleDetectorSweepService creates a new sweep service.
func NewDrixyRuleDetectorSweepService() *DrixyRuleDetectorSweepService {
	return &DrixyRuleDetectorSweepService{}
}

// SweepDiffAddedLines scans added lines in a patch against all active compiled rule detectors.
func (s *DrixyRuleDetectorSweepService) SweepDiffAddedLines(
	ctx context.Context,
	rules []interfaces.DrixyRule,
	filePath string,
	addedLines []string,
) []DetectorMatch {
	var matches []DetectorMatch

	type compiledRule struct {
		rule  interfaces.DrixyRule
		regex *regexp.Regexp
	}

	var compiled []compiledRule
	for _, r := range rules {
		if r.Status != interfaces.DrixyRulesStatusActive || r.Detector == nil || r.Detector.Pattern == "" {
			continue
		}
		if re, err := regexp.Compile(r.Detector.Pattern); err == nil {
			compiled = append(compiled, compiledRule{rule: r, regex: re})
		}
	}

	if len(compiled) == 0 {
		return nil
	}

	for lineIdx, line := range addedLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, cr := range compiled {
			if cr.regex.MatchString(line) {
				matches = append(matches, DetectorMatch{
					RuleUUID:    cr.rule.UUID,
					RuleTitle:   cr.rule.Title,
					Severity:    interfaces.ResolveDrixyRuleSeverityLevel(&cr.rule),
					FilePath:    filePath,
					LineNumber:  lineIdx + 1,
					LineContent: trimmed,
					Reason:      cr.rule.Rule,
				})
			}
		}
	}

	return matches
}

// ConvertToFindings transforms detector matches into standard CodeFinding objects.
func (s *DrixyRuleDetectorSweepService) ConvertToFindings(matches []DetectorMatch, reviewID uuid.UUID) []models.CodeFinding {
	findings := make([]models.CodeFinding, len(matches))
	for i, m := range matches {
		findings[i] = models.CodeFinding{
			ID:          uuid.New(),
			ReviewID:    reviewID,
			Title:       m.RuleTitle,
			Severity:    m.Severity,
			FilePath:    m.FilePath,
			StartLine:   m.LineNumber,
			EndLine:     m.LineNumber,
			Description: m.Reason,
			Category:    "MECHANICAL_SWEEP",
			CreatedAt:   time.Now().UTC(),
		}
	}
	return findings
}
