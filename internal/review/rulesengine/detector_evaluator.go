// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// DetectorEvaluator executes deterministic regex and token checks directly on diff hunks.
type DetectorEvaluator struct{}

// NewDetectorEvaluator constructs a mechanical detector evaluator.
func NewDetectorEvaluator() *DetectorEvaluator {
	return &DetectorEvaluator{}
}

// CompileRuleDetector compiles the raw regex pattern and optional negative regex pattern on a rule.
func CompileRuleDetector(detector *CompiledRuleDetector) error {
	if detector == nil || strings.TrimSpace(detector.Pattern) == "" {
		return fmt.Errorf("detector pattern cannot be empty")
	}

	pattern := detector.Pattern
	flags := detector.Flags
	var prefix string
	if strings.Contains(flags, "i") {
		prefix += "i"
	}
	if strings.Contains(flags, "m") {
		prefix += "m"
	}
	if prefix != "" {
		pattern = "(?" + prefix + ")" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid detector regex '%s': %w", detector.Pattern, err)
	}
	detector.CompiledRegex = re

	if strings.TrimSpace(detector.NegativePattern) != "" {
		negPattern := detector.NegativePattern
		if prefix != "" {
			negPattern = "(?" + prefix + ")" + negPattern
		}
		negRe, err := regexp.Compile(negPattern)
		if err != nil {
			return fmt.Errorf("invalid negative detector regex '%s': %w", detector.NegativePattern, err)
		}
		detector.CompiledNegative = negRe
	}

	return nil
}

// matchPathGlob evaluates whether a file matches path globs with double-star wildcard support.
func matchPathGlob(pattern, filePath string) bool {
	pattern = filepath.ToSlash(filepath.Clean(pattern))
	filePath = filepath.ToSlash(filepath.Clean(filePath))
	filePath = strings.TrimPrefix(strings.TrimPrefix(filePath, "./"), "/")
	pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, "./"), "/")

	if pattern == "" || pattern == "*" || pattern == "**" {
		return true
	}

	var regexParts []string
	segments := strings.Split(pattern, "/")
	for i, seg := range segments {
		if seg == "**" {
			if i == len(segments)-1 {
				regexParts = append(regexParts, ".*")
			} else {
				regexParts = append(regexParts, "(?:.*/)?")
			}
		} else {
			escaped := regexp.QuoteMeta(seg)
			escaped = strings.ReplaceAll(escaped, "\\*", "[^/]*")
			escaped = strings.ReplaceAll(escaped, "\\?", "[^/]")
			if i > 0 && segments[i-1] == "**" {
				regexParts = append(regexParts, escaped)
			} else {
				if i > 0 {
					regexParts = append(regexParts, "/"+escaped)
				} else {
					regexParts = append(regexParts, escaped)
				}
			}
		}
	}

	finalRegex := "^" + strings.Join(regexParts, "") + "$"
	matched, err := regexp.MatchString(finalRegex, filePath)
	if err != nil {
		return false
	}
	return matched
}

// EvaluateRules evaluates all mechanical rules across a set of pull request diff patches.
func (e *DetectorEvaluator) EvaluateRules(
	ctx context.Context,
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
	rules []*DrixyRule,
	patches []*diff.FilePatch,
) []models.CodeFinding {
	if len(rules) == 0 || len(patches) == 0 {
		return nil
	}

	var findings []models.CodeFinding

	for _, rule := range rules {
		if rule.Status != StatusActive || rule.Detector == nil {
			continue
		}

		if rule.Detector.CompiledRegex == nil {
			if err := CompileRuleDetector(rule.Detector); err != nil {
				continue
			}
		}

		for _, patch := range patches {
			targetPath := patch.NewPath
			if targetPath == "" {
				targetPath = patch.OldPath
			}
			if targetPath == "" {
				continue
			}

			// Path glob filter check
			if len(rule.PathGlobs) > 0 {
				matched := false
				for _, glob := range rule.PathGlobs {
					if matchPathGlob(glob, targetPath) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}

			// Evaluate line additions in hunks
			for _, hunk := range patch.Hunks {
				for _, line := range hunk.Lines {
					if line.Type != diff.LineAddition {
						continue
					}

					content := line.Content
					if rule.Detector.CompiledRegex.MatchString(content) {
						if rule.Detector.CompiledNegative != nil && rule.Detector.CompiledNegative.MatchString(content) {
							// Negative pattern matched -> skip false positive
							continue
						}

						lineNo := line.NewLineNo
						if lineNo <= 0 {
							lineNo = 1
						}

						hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d", rule.Slug, targetPath, lineNo)))
						fp := hex.EncodeToString(hash[:16])

						desc := rule.Description
						if rule.Detector.Reason != "" {
							desc = fmt.Sprintf("%s\n\n*Detection Trigger*: %s", desc, rule.Detector.Reason)
						}

						findings = append(findings, models.CodeFinding{
							ID:            uuid.New(),
							ReviewID:      reviewID,
							WorkspaceID:   workspaceID,
							FilePath:      targetPath,
							StartLine:     lineNo,
							EndLine:       lineNo,
							Severity:      rule.Severity,
							Category:      "drixy_rules",
							Title:         rule.Title,
							Description:   desc,
							Remediation:   rule.RemediationHint,
							SuggestedDiff: rule.RemediationHint,
							Fingerprint:   fp,
							CreatedAt:     time.Now().UTC(),
						})
					}
				}
			}
		}
	}

	return findings
}
