// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// GroupFindingsByFile clusters code review findings by their target relative file path.
func GroupFindingsByFile(findings []models.CodeFinding) map[string][]models.CodeFinding {
	grouped := make(map[string][]models.CodeFinding)
	for _, f := range findings {
		grouped[f.FilePath] = append(grouped[f.FilePath], f)
	}
	return grouped
}

// FileFindingStats holds severity distribution counts for a file or changeset.
type FileFindingStats struct {
	Critical int
	High     int
	Medium   int
	Low      int
	Info     int
}

// GetFileStats calculates severity counts for a slice of findings.
func GetFileStats(findings []models.CodeFinding) FileFindingStats {
	var stats FileFindingStats
	for _, f := range findings {
		switch strings.ToUpper(string(f.Severity)) {
		case "CRITICAL":
			stats.Critical++
		case "HIGH":
			stats.High++
		case "MEDIUM":
			stats.Medium++
		case "LOW":
			stats.Low++
		default:
			stats.Info++
		}
	}
	return stats
}

// FormatCategoryBadge returns an abbreviated category tag for clean display.
func FormatCategoryBadge(category string) string {
	categoryMap := map[string]string{
		"security_vulnerability": "security",
		"security":               "security",
		"performance":            "perf",
		"code_quality":           "quality",
		"best_practices":         "practices",
		"style":                  "style",
		"bug":                    "bug",
		"complexity":             "complex",
		"maintainability":        "maintain",
	}
	if mapped, ok := categoryMap[strings.ToLower(category)]; ok {
		return mapped
	}
	return category
}

// GenerateFixPrompt formats a target file's issues as a prompt for AI coding agents.
func GenerateFixPrompt(file string, findings []models.CodeFinding) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Fix the following issues in %s:\n\n", file))

	for i, finding := range findings {
		b.WriteString(fmt.Sprintf("%d. %s at line %d\n", i+1, strings.ToUpper(string(finding.Severity)), finding.StartLine))
		b.WriteString(fmt.Sprintf("   %s: %s\n", finding.Title, finding.Description))

		if finding.Remediation != "" {
			b.WriteString(fmt.Sprintf("   Remediation: %s\n", finding.Remediation))
		}
		if finding.SuggestedDiff != "" {
			b.WriteString(fmt.Sprintf("   Suggested Diff:\n%s\n", finding.SuggestedDiff))
		}
		b.WriteString("\n")
	}

	plural := "issue"
	if len(findings) != 1 {
		plural = "issues"
	}
	b.WriteString(fmt.Sprintf("Please fix these %d %s in %s.", len(findings), plural, file))
	return b.String()
}

// GenerateFixPromptAll formats all findings across all files as a unified multi-file prompt.
func GenerateFixPromptAll(findingsByFile map[string][]models.CodeFinding) string {
	totalFindings := 0
	for _, list := range findingsByFile {
		totalFindings += len(list)
	}
	fileCount := len(findingsByFile)

	var b strings.Builder
	plural := "issues"
	if totalFindings == 1 {
		plural = "issue"
	}
	fPlural := "files"
	if fileCount == 1 {
		fPlural = "file"
	}

	b.WriteString(fmt.Sprintf("Fix the following %d %s across %d %s.\n\n", totalFindings, plural, fileCount, fPlural))
	b.WriteString("Work file-by-file in the order below. For each file, address every listed issue before moving on.\n\n")

	// Sort file keys for deterministic output
	files := make([]string, 0, len(findingsByFile))
	for f := range findingsByFile {
		files = append(files, f)
	}
	sort.Strings(files)

	for idx, file := range files {
		list := findingsByFile[file]
		iPlural := "issues"
		if len(list) == 1 {
			iPlural = "issue"
		}
		b.WriteString(fmt.Sprintf("## File %d/%d: %s (%d %s)\n\n", idx+1, fileCount, file, len(list), iPlural))

		for i, finding := range list {
			b.WriteString(fmt.Sprintf("%d. %s at line %d\n", i+1, strings.ToUpper(string(finding.Severity)), finding.StartLine))
			b.WriteString(fmt.Sprintf("   %s: %s\n", finding.Title, finding.Description))
			if finding.Remediation != "" {
				b.WriteString(fmt.Sprintf("   Remediation: %s\n", finding.Remediation))
			}
			if finding.SuggestedDiff != "" {
				b.WriteString(fmt.Sprintf("   Suggested Diff:\n%s\n", finding.SuggestedDiff))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// GetFixableFindings filters findings that have automated suggested diffs.
func GetFixableFindings(findings []models.CodeFinding) []models.CodeFinding {
	var fixable []models.CodeFinding
	for _, f := range findings {
		if strings.TrimSpace(f.SuggestedDiff) != "" {
			fixable = append(fixable, f)
		}
	}
	return fixable
}
