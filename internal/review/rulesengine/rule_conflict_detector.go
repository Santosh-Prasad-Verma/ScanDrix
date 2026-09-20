// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ConflictSeverity categorizes the impact of an identified rule collision.
type ConflictSeverity string

const (
	ConflictCritical ConflictSeverity = "CRITICAL" // Direct contradiction (e.g. MUST vs MUST NOT)
	ConflictWarning  ConflictSeverity = "WARNING"  // Shadowed rule or severity mismatch
	ConflictInfo     ConflictSeverity = "INFO"     // Redundant/duplicate rule patterns
)

// ConflictKind defines the architectural classification of the collision.
type ConflictKind string

const (
	KindContradiction ConflictKind = "CONTRADICTION"
	KindShadowing     ConflictKind = "SHADOWING"
	KindRedundancy    ConflictKind = "REDUNDANCY"
	KindScopeOverlap  ConflictKind = "SCOPE_OVERLAP"
)

// RuleConflict describes an identified collision between two custom rules.
type RuleConflict struct {
	ConflictID          string           `json:"conflict_id"`
	Kind                ConflictKind     `json:"kind"`
	Severity            ConflictSeverity `json:"severity"`
	RuleAID             uuid.UUID        `json:"rule_a_id"`
	RuleAName           string           `json:"rule_a_name"`
	RuleBID             uuid.UUID        `json:"rule_b_id"`
	RuleBName           string           `json:"rule_b_name"`
	OverlappingGlobs    []string         `json:"overlapping_globs"`
	Explanation         string           `json:"explanation"`
	SuggestedResolution string           `json:"suggested_resolution"`
}

// RuleConflictReport summarizes all detected rule collisions across a repository or organization.
type RuleConflictReport struct {
	TotalRulesEvaluated int            `json:"total_rules_evaluated"`
	TotalConflicts      int            `json:"total_conflicts"`
	CriticalCount       int            `json:"critical_count"`
	WarningCount        int            `json:"warning_count"`
	InfoCount           int            `json:"info_count"`
	Conflicts           []RuleConflict `json:"conflicts"`
}

// RuleConflictDetector inspects rule collections for contradictions, redundancies, and shadowing.
type RuleConflictDetector struct{}

// NewRuleConflictDetector constructs a rule conflict detector.
func NewRuleConflictDetector() *RuleConflictDetector {
	return &RuleConflictDetector{}
}

// InspectRuleCollection cross-examines a set of rules and surfaces all conflicting invariants.
func (d *RuleConflictDetector) InspectRuleCollection(rules []*DrixyRuleAtom) RuleConflictReport {
	report := RuleConflictReport{
		TotalRulesEvaluated: len(rules),
		Conflicts:           make([]RuleConflict, 0),
	}

	if len(rules) < 2 {
		return report
	}

	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			rA := rules[i]
			rB := rules[j]

			// Check if rule paths can ever overlap
			overlappingGlobs := findOverlappingGlobs(rA.PathGlobs, rB.PathGlobs)
			if len(overlappingGlobs) == 0 && (len(rA.PathGlobs) > 0 || len(rB.PathGlobs) > 0) {
				// No overlap possible
				continue
			}

			// 1. Contradiction check: Opposing normative levels with overlapping detectors or titles
			if isOpposingNormativeLevel(rA.NormativeLevel, rB.NormativeLevel) {
				if hasSemanticOrPatternOverlap(rA, rB) {
					conflict := RuleConflict{
						ConflictID:       fmt.Sprintf("conflict-%s-%s-contra", rA.ID, rB.ID),
						Kind:             KindContradiction,
						Severity:         ConflictCritical,
						RuleAID:          rA.ParentRuleID,
						RuleAName:        rA.Title,
						RuleBID:          rB.ParentRuleID,
						RuleBName:        rB.Title,
						OverlappingGlobs: overlappingGlobs,
						Explanation: fmt.Sprintf("Contradictory directives: [%s] enforces %s while [%s] enforces %s over matching scopes.",
							rA.Title, rA.NormativeLevel, rB.Title, rB.NormativeLevel),
						SuggestedResolution: fmt.Sprintf("Harmonize normative levels or partition path scopes so [%s] and [%s] do not evaluate the same files.",
							rA.Title, rB.Title),
					}
					report.Conflicts = append(report.Conflicts, conflict)
					report.CriticalCount++
					continue
				}
			}

			// 2. Redundancy check: Identical detectors and matching normative direction
			if rA.Detector != nil && rB.Detector != nil && rA.Detector.Pattern != "" {
				if strings.TrimSpace(rA.Detector.Pattern) == strings.TrimSpace(rB.Detector.Pattern) {
					conflict := RuleConflict{
						ConflictID:       fmt.Sprintf("conflict-%s-%s-redundant", rA.ID, rB.ID),
						Kind:             KindRedundancy,
						Severity:         ConflictInfo,
						RuleAID:          rA.ParentRuleID,
						RuleAName:        rA.Title,
						RuleBID:          rB.ParentRuleID,
						RuleBName:        rB.Title,
						OverlappingGlobs: overlappingGlobs,
						Explanation: fmt.Sprintf("Exact regex detector redundancy: Both [%s] and [%s] compile identical pattern `%s`.",
							rA.Title, rB.Title, rA.Detector.Pattern),
						SuggestedResolution: "Consolidate duplicate rules into a single canonical rule definition.",
					}
					report.Conflicts = append(report.Conflicts, conflict)
					report.InfoCount++
					continue
				}
			}

			// 3. Shadowing / Severity Mismatch: Same title or spec with different severities
			if strings.EqualFold(strings.TrimSpace(rA.Title), strings.TrimSpace(rB.Title)) {
				if rA.Severity != rB.Severity {
					conflict := RuleConflict{
						ConflictID:       fmt.Sprintf("conflict-%s-%s-shadow", rA.ID, rB.ID),
						Kind:             KindShadowing,
						Severity:         ConflictWarning,
						RuleAID:          rA.ParentRuleID,
						RuleAName:        rA.Title,
						RuleBID:          rB.ParentRuleID,
						RuleBName:        rB.Title,
						OverlappingGlobs: overlappingGlobs,
						Explanation: fmt.Sprintf("Rule shadowing with severity mismatch: [%s] has severity %s while [%s] has severity %s.",
							rA.Title, rA.Severity, rB.Title, rB.Severity),
						SuggestedResolution: "Align severities or delete the lower-priority shadowed rule definition.",
					}
					report.Conflicts = append(report.Conflicts, conflict)
					report.WarningCount++
				}
			}
		}
	}

	report.TotalConflicts = len(report.Conflicts)
	return report
}

func isOpposingNormativeLevel(a, b RFC2119Level) bool {
	isNegativeA := (a == NormativeMustNot || a == NormativeShallNot || a == NormativeNever || a == NormativeShouldNot)
	isPositiveA := (a == NormativeMust || a == NormativeRequired || a == NormativeShall || a == NormativeAlways)

	isNegativeB := (b == NormativeMustNot || b == NormativeShallNot || b == NormativeNever || b == NormativeShouldNot)
	isPositiveB := (b == NormativeMust || b == NormativeRequired || b == NormativeShall || b == NormativeAlways)

	return (isNegativeA && isPositiveB) || (isPositiveA && isNegativeB)
}

func hasSemanticOrPatternOverlap(rA, rB *DrixyRuleAtom) bool {
	if rA.Detector != nil && rB.Detector != nil && rA.Detector.Pattern != "" && rB.Detector.Pattern != "" {
		if strings.Contains(rA.Detector.Pattern, rB.Detector.Pattern) || strings.Contains(rB.Detector.Pattern, rA.Detector.Pattern) {
			return true
		}
	}

	wordsA := strings.Fields(strings.ToLower(rA.Title + " " + rA.Spec))
	wordsB := strings.Fields(strings.ToLower(rB.Title + " " + rB.Spec))

	setA := make(map[string]struct{}, len(wordsA))
	for _, w := range wordsA {
		if len(w) > 4 {
			setA[w] = struct{}{}
		}
	}

	common := 0
	for _, w := range wordsB {
		if _, exists := setA[w]; exists {
			common++
		}
	}

	return common >= 3
}

func findOverlappingGlobs(globsA, globsB []string) []string {
	if len(globsA) == 0 && len(globsB) == 0 {
		return []string{"**/*"}
	}
	if len(globsA) == 0 {
		return globsB
	}
	if len(globsB) == 0 {
		return globsA
	}

	var overlaps []string
	for _, a := range globsA {
		for _, b := range globsB {
			if globsOverlap(a, b) {
				overlaps = append(overlaps, fmt.Sprintf("%s ∩ %s", a, b))
			}
		}
	}

	return overlaps
}

func globsOverlap(globA, globB string) bool {
	cleanA := strings.ToLower(filepath.ToSlash(globA))
	cleanB := strings.ToLower(filepath.ToSlash(globB))

	if cleanA == cleanB || cleanA == "**/*" || cleanB == "**/*" || cleanA == "*" || cleanB == "*" {
		return true
	}

	extA := filepath.Ext(cleanA)
	extB := filepath.Ext(cleanB)
	if extA != "" && extB != "" && extA != extB {
		return false
	}

	dirA := filepath.Dir(cleanA)
	dirB := filepath.Dir(cleanB)
	if dirA != "." && dirB != "." && dirA != dirB && !strings.Contains(dirA, "**") && !strings.Contains(dirB, "**") {
		return false
	}

	return true
}
