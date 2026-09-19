package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/internal/try"
	"github.com/scandrix/backend/pkg/models"
)

// PipelineFile represents a prepared file for AST and rule analysis.
type PipelineFile struct {
	Path      string
	OldPath   *string
	Status    string
	Additions int
	Deletions int
	Patch     string
	Content   string
}

// PrepareCliFiles processes input diff and files into structured pipeline files.
func PrepareCliFiles(input domain.CliReviewInput) []PipelineFile {
	var out []PipelineFile

	if input.Config != nil && len(input.Config.Files) > 0 {
		for _, f := range input.Config.Files {
			add, del := countPatchChanges(f.Diff)
			out = append(out, PipelineFile{
				Path:      f.Path,
				Status:    f.Status,
				Additions: add,
				Deletions: del,
				Patch:     f.Diff,
				Content:   f.Content,
			})
		}
		return out
	}

	diffFiles := try.ParseUnifiedDiff(input.Diff)
	for _, df := range diffFiles {
		out = append(out, PipelineFile{
			Path:      df.Path,
			OldPath:   df.OldPath,
			Status:    string(df.Status),
			Additions: df.Additions,
			Deletions: df.Deletions,
			Patch:     input.Diff, // raw hunk patch
			Content:   "",
		})
	}
	return out
}

func countPatchChanges(patch string) (additions, deletions int) {
	lines := strings.Split(patch, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			additions++
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			deletions++
		}
	}
	return
}

// EvaluateRulesAgainstFiles evaluates AST rules on prepared files.
func EvaluateRulesAgainstFiles(
	evaluator *rules.Evaluator,
	files []PipelineFile,
	cfg *domain.CliReviewConfig,
) []domain.CliReviewIssue {
	if evaluator == nil {
		return nil
	}

	var allPatches []*diff.FilePatch
	for _, f := range files {
		if strings.Contains(f.Patch, "diff --git") {
			patches, err := diff.ParseUnifiedDiff(strings.NewReader(f.Patch))
			if err == nil && len(patches) > 0 {
				allPatches = append(allPatches, patches...)
				continue
			}
		}

		// Fallback: construct FilePatch directly for standalone snippets or file inputs
		patch := &diff.FilePatch{
			NewPath:   f.Path,
			Additions: f.Additions,
			Deletions: f.Deletions,
		}
		var lines []diff.DiffLine
		rawLines := strings.Split(f.Patch, "\n")
		lineNo := 1
		for _, l := range rawLines {
			if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
				lines = append(lines, diff.DiffLine{
					Type:      diff.LineAddition,
					NewLineNo: lineNo,
					Content:   l[1:],
				})
				lineNo++
			} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				lines = append(lines, diff.DiffLine{
					Type:    diff.LineDeletion,
					Content: l[1:],
				})
			} else {
				content := l
				if strings.HasPrefix(l, " ") {
					content = l[1:]
				}
				lines = append(lines, diff.DiffLine{
					Type:      diff.LineContext,
					NewLineNo: lineNo,
					Content:   content,
				})
				lineNo++
			}
		}
		if len(lines) > 0 {
			patch.Hunks = []diff.Hunk{
				{
					NewStart: 1,
					NewLines: len(lines),
					Lines:    lines,
				},
			}
		}
		allPatches = append(allPatches, patch)
	}

	findings := evaluator.EvaluatePatches(uuid.Nil, uuid.Nil, allPatches)
	var issues []domain.CliReviewIssue

	for _, finding := range findings {
		// Check rule toggles if provided
		if cfg != nil && cfg.Rules != nil {
			cat := strings.ToLower(finding.Category)
			if cat == "security" && !cfg.Rules.Security {
				continue
			}
			if cat == "performance" && !cfg.Rules.Performance {
				continue
			}
			if cat == "style" && !cfg.Rules.Style {
				continue
			}
			if cat == "best_practices" && !cfg.Rules.BestPractices {
				continue
			}
		}

		// Check severity filter
		if cfg != nil && cfg.Severity != "" {
			if !severityMeetsThreshold(finding.Severity, cfg.Severity) {
				continue
			}
		}

		endLine := finding.EndLine
		issues = append(issues, domain.CliReviewIssue{
			File:           finding.FilePath,
			Line:           finding.StartLine,
			EndLine:        &endLine,
			Severity:       string(finding.Severity),
			Category:       finding.Category,
			Message:        finding.Description,
			Suggestion:     finding.Remediation,
			Recommendation: finding.Remediation,
			RuleID:         finding.Title,
			Fixable:        finding.SuggestedDiff != "",
		})
	}

	return issues
}

func severityMeetsThreshold(actual models.FindingSeverity, threshold string) bool {
	rank := map[models.FindingSeverity]int{
		models.SeverityInfo:     1,
		models.SeverityLow:      2,
		models.SeverityMedium:   3,
		models.SeverityHigh:     4,
		models.SeverityCritical: 5,
	}

	threshRank := map[string]int{
		"info":     1,
		"low":      2,
		"medium":   3,
		"high":     4,
		"critical": 5,
	}

	tVal, ok := threshRank[strings.ToLower(threshold)]
	if !ok {
		return true
	}
	return rank[actual] >= tVal
}

// FormatCliOutput formats findings into the final CliReviewResponse.
func FormatCliOutput(issues []domain.CliReviewIssue, filesCount int, startTime time.Time) domain.CliReviewResponse {
	duration := time.Since(startTime).Milliseconds()
	summary := generateSummary(issues, filesCount)

	return domain.CliReviewResponse{
		Summary:       summary,
		Issues:        issues,
		FilesAnalyzed: filesCount,
		Duration:      duration,
	}
}

func generateSummary(issues []domain.CliReviewIssue, filesCount int) string {
	if len(issues) == 0 {
		return fmt.Sprintf("ScanDrix: No issues found across %d file(s). All security and quality checks passed.", filesCount)
	}

	var crit, high, med, low int
	for _, i := range issues {
		switch strings.ToLower(i.Severity) {
		case "critical":
			crit++
		case "high":
			high++
		case "medium":
			med++
		default:
			low++
		}
	}

	return fmt.Sprintf(
		"ScanDrix: Found %d issue(s) across %d file(s): %d critical, %d high, %d medium, %d low/info.",
		len(issues), filesCount, crit, high, med, low,
	)
}
