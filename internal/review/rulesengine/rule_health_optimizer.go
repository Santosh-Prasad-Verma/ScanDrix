// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RuleHealthGrade ranks the production utility and developer reception of a rule.
type RuleHealthGrade string

const (
	HealthExcellent RuleHealthGrade = "EXCELLENT" // >80% acceptance, <10% false positive
	HealthHealthy   RuleHealthGrade = "HEALTHY"   // >60% acceptance, <20% false positive
	HealthDegraded  RuleHealthGrade = "DEGRADED"  // 20-40% false positive, needs negative pattern tuning
	HealthToxic     RuleHealthGrade = "TOXIC"     // >40% false positive, recommended auto-pause
)

// DeveloperRuleFeedback models a developer's reaction to a rule finding on a PR.
type DeveloperRuleFeedback struct {
	RuleID       uuid.UUID `json:"rule_id"`
	ReviewID     uuid.UUID `json:"review_id"`
	FilePath     string    `json:"file_path"`
	LineNumber   int       `json:"line_number"`
	CodeSnippet  string    `json:"code_snippet"`
	FeedbackType string    `json:"feedback_type"` // "ACCEPTED", "THUMBS_UP", "DISMISSED", "FALSE_POSITIVE"
	Reason       string    `json:"reason,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// RuleHealthReport summarizes performance and automated tuning recommendations.
type RuleHealthReport struct {
	RuleID               uuid.UUID       `json:"rule_id"`
	TotalEvaluations     int             `json:"total_evaluations"`
	TotalFindingsEmitted int             `json:"total_findings_emitted"`
	AcceptedCount        int             `json:"accepted_count"`
	FalsePositiveCount   int             `json:"false_positive_count"`
	DismissedCount       int             `json:"dismissed_count"`
	AcceptanceRate       float64         `json:"acceptance_rate"`
	FalsePositiveRate    float64         `json:"false_positive_rate"`
	NoiseRatio           float64         `json:"noise_ratio"`
	Grade                RuleHealthGrade `json:"grade"`
	AutoAction           string          `json:"auto_action,omitempty"`
	SynthesizedNegative  string          `json:"synthesized_negative,omitempty"`
}

// RuleHealthOptimizer tracks feedback telemetry, calculates noise ratios, and synthesizes negative regex patterns.
type RuleHealthOptimizer struct {
	toxicThreshold float64
}

// NewRuleHealthOptimizer constructs an optimizer.
func NewRuleHealthOptimizer(toxicThreshold ...float64) *RuleHealthOptimizer {
	thresh := 0.40 // 40% false positive triggers auto-demotion
	if len(toxicThreshold) > 0 && toxicThreshold[0] > 0 {
		thresh = toxicThreshold[0]
	}
	return &RuleHealthOptimizer{
		toxicThreshold: thresh,
	}
}

// EvaluateRuleHealth analyzes a batch of developer feedback for a rule and determines grade and tuning actions.
func (o *RuleHealthOptimizer) EvaluateRuleHealth(
	ctx context.Context,
	ruleID uuid.UUID,
	feedbacks []DeveloperRuleFeedback,
	currentRule *SynthesizedRuleDefinition,
) RuleHealthReport {
	report := RuleHealthReport{
		RuleID:           ruleID,
		TotalEvaluations: len(feedbacks),
	}

	if len(feedbacks) == 0 {
		report.Grade = HealthHealthy
		return report
	}

	var falsePositiveSnippets []string

	for _, fb := range feedbacks {
		report.TotalFindingsEmitted++
		switch strings.ToUpper(fb.FeedbackType) {
		case "ACCEPTED", "THUMBS_UP":
			report.AcceptedCount++
		case "FALSE_POSITIVE":
			report.FalsePositiveCount++
			if fb.CodeSnippet != "" {
				falsePositiveSnippets = append(falsePositiveSnippets, fb.CodeSnippet)
			}
		case "DISMISSED":
			report.DismissedCount++
			if strings.Contains(strings.ToLower(fb.Reason), "false positive") || strings.Contains(strings.ToLower(fb.Reason), "incorrect") {
				report.FalsePositiveCount++
				if fb.CodeSnippet != "" {
					falsePositiveSnippets = append(falsePositiveSnippets, fb.CodeSnippet)
				}
			}
		}
	}

	totalDecisions := report.AcceptedCount + report.FalsePositiveCount + report.DismissedCount
	if totalDecisions > 0 {
		report.AcceptanceRate = float64(report.AcceptedCount) / float64(totalDecisions)
		report.FalsePositiveRate = float64(report.FalsePositiveCount) / float64(totalDecisions)
		report.NoiseRatio = float64(report.FalsePositiveCount+report.DismissedCount) / float64(totalDecisions)
	}

	// Determine grade
	switch {
	case report.FalsePositiveRate >= o.toxicThreshold && totalDecisions >= 5:
		report.Grade = HealthToxic
		report.AutoAction = "AUTO_DEMOTE_TO_DRAFT"
	case report.FalsePositiveRate >= 0.20 && totalDecisions >= 3:
		report.Grade = HealthDegraded
		report.AutoAction = "SYNTHESIZE_NEGATIVE_REGEX"
	case report.AcceptanceRate >= 0.80 && report.FalsePositiveRate < 0.10:
		report.Grade = HealthExcellent
	default:
		report.Grade = HealthHealthy
	}

	// Synthesize negative regex suppression pattern from false positive snippets
	if len(falsePositiveSnippets) > 0 {
		report.SynthesizedNegative = o.SynthesizeNegativePattern(falsePositiveSnippets, currentRule)
	}

	return report
}

// SynthesizeNegativePattern generates a negative suppression regex to prevent recurring false positives.
func (o *RuleHealthOptimizer) SynthesizeNegativePattern(snippets []string, currentRule *SynthesizedRuleDefinition) string {
	if len(snippets) == 0 {
		return ""
	}

	// Extract distinctive identifiers or prefixes
	tokensMap := make(map[string]int)
	identRegex := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_\.]{3,}\b`)

	for _, s := range snippets {
		matches := identRegex.FindAllString(s, -1)
		for _, m := range matches {
			lower := strings.ToLower(m)
			// Skip common keywords
			if isCommonKeyword(lower) {
				continue
			}
			tokensMap[lower]++
		}
	}

	var candidates []string
	for token, count := range tokensMap {
		if count >= 1 {
			candidates = append(candidates, regexp.QuoteMeta(token))
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// Take top 3 most common tokens
	sort.Slice(candidates, func(i, j int) bool {
		return tokensMap[candidates[i]] > tokensMap[candidates[j]]
	})

	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	synthesized := fmt.Sprintf(`(?i)(%s)`, strings.Join(candidates, "|"))

	if currentRule != nil && currentRule.NegativePattern != "" {
		// Merge with existing negative pattern
		return fmt.Sprintf(`(?:%s|%s)`, currentRule.NegativePattern, synthesized)
	}

	return synthesized
}

func isCommonKeyword(kw string) bool {
	switch kw {
	case "func", "return", "error", "nil", "string", "true", "false", "type", "struct",
		"const", "var", "import", "package", "context", "interface", "class", "public",
		"private", "async", "await", "function", "let":
		return true
	default:
		return false
	}
}
