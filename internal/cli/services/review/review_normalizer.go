// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package review

import (
	"strings"
	"time"

	"github.com/scandrix/backend/internal/cli/types"
	"github.com/scandrix/backend/pkg/models"
)

// NormalizeSeverity maps arbitrary severity strings into canonical types.Severity.
func NormalizeSeverity(raw string) types.Severity {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "critical":
		return types.SeverityCritical
	case "high", "error":
		return types.SeverityError
	case "medium", "warning":
		return types.SeverityWarning
	default:
		return types.SeverityInfo
	}
}

// ToFindingSeverity converts a types.Severity to models.FindingSeverity.
func ToFindingSeverity(sev types.Severity) models.FindingSeverity {
	switch sev {
	case types.SeverityCritical:
		return models.SeverityCritical
	case types.SeverityError:
		return models.SeverityHigh
	case types.SeverityWarning:
		return models.SeverityMedium
	default:
		return models.SeverityLow
	}
}

// RawAPISuggestion represents an incoming suggestion from the ScanDrix API.
type RawAPISuggestion struct {
	FilePath             string `json:"filePath,omitempty"`
	RelevantFile         string `json:"relevantFile,omitempty"`
	RelevantLinesStart   int    `json:"relevantLinesStart,omitempty"`
	RelevantLinesEnd     int    `json:"relevantLinesEnd,omitempty"`
	Severity             string `json:"severity,omitempty"`
	SuggestionContent    string `json:"suggestionContent,omitempty"`
	OneSentenceSummary   string `json:"oneSentenceSummary,omitempty"`
	Label                string `json:"label,omitempty"`
	SuggestedDiff        string `json:"suggestedDiff,omitempty"`
}

// RawAPISuggestionsObject represents a container for file and PR level suggestions.
type RawAPISuggestionsObject struct {
	Files   []RawAPISuggestion `json:"files"`
	PRLevel []RawAPISuggestion `json:"prLevel"`
}

// NormalizeRawSuggestions converts API suggestions into structured ReviewIssues.
func NormalizeRawSuggestions(rawFiles, rawPR []RawAPISuggestion) []types.ReviewIssue {
	var issues []types.ReviewIssue

	for _, s := range rawFiles {
		filePath := s.FilePath
		if filePath == "" {
			filePath = s.RelevantFile
		}
		if filePath == "" {
			filePath = "unknown"
		}

		line := s.RelevantLinesStart
		if line <= 0 {
			line = 1
		}

		sev := NormalizeSeverity(s.Severity)
		issue := types.ReviewIssue{
			ID:          s.Label,
			File:        filePath,
			Line:        line,
			EndLine:     s.RelevantLinesEnd,
			Severity:    sev,
			Category:    s.Label,
			Message:     s.SuggestionContent,
			Suggestion:  s.OneSentenceSummary,
			RuleID:      s.Label,
			Fixable:     s.SuggestedDiff != "",
		}

		if s.SuggestedDiff != "" {
			issue.Fix = &types.CodeFix{
				Explanation: s.OneSentenceSummary,
				NewCode:     s.SuggestedDiff,
				StartLine:   line,
				EndLine:     s.RelevantLinesEnd,
			}
		}

		issues = append(issues, issue)
	}

	for _, s := range rawPR {
		sev := NormalizeSeverity(s.Severity)
		issue := types.ReviewIssue{
			ID:         s.Label,
			File:       "PR",
			Line:       0,
			Severity:   sev,
			Category:   "pr-level",
			Message:    s.SuggestionContent,
			Suggestion: s.OneSentenceSummary,
			RuleID:     s.Label,
		}
		issues = append(issues, issue)
	}

	return issues
}

// CalculateReviewStats aggregates issue statistics across severities.
func CalculateReviewStats(issues []types.ReviewIssue) types.ReviewStats {
	var stats types.ReviewStats
	stats.TotalIssues = len(issues)

	for _, iss := range issues {
		switch iss.Severity {
		case types.SeverityCritical:
			stats.CriticalCount++
		case types.SeverityError:
			stats.ErrorCount++
		case types.SeverityWarning:
			stats.WarningCount++
		default:
			stats.InfoCount++
		}
		if iss.Fixable || iss.Fix != nil {
			stats.FixableCount++
		}
	}

	return stats
}

// BuildReviewResult assembles the complete canonical ReviewResult.
func BuildReviewResult(reviewID, summary string, issues []types.ReviewIssue, filesAnalyzed int, duration time.Duration) types.ReviewResult {
	stats := CalculateReviewStats(issues)
	status := "passed"
	exitCode := 0
	isBlocking := false

	if stats.CriticalCount > 0 || stats.ErrorCount > 0 {
		status = "failed"
		exitCode = 1
		isBlocking = true
	} else if stats.WarningCount > 0 {
		status = "warning"
	}

	if filesAnalyzed <= 0 {
		uniqueFiles := make(map[string]bool)
		for _, iss := range issues {
			if iss.File != "PR" && iss.File != "" {
				uniqueFiles[iss.File] = true
			}
		}
		filesAnalyzed = len(uniqueFiles)
	}

	return types.ReviewResult{
		ReviewID:      reviewID,
		Status:        status,
		Summary:       summary,
		FilesAnalyzed: filesAnalyzed,
		Issues:        issues,
		Stats:         stats,
		Duration:      duration,
		DurationMs:    duration.Milliseconds(),
		IsBlocking:    isBlocking,
		ExitCode:      exitCode,
	}
}
