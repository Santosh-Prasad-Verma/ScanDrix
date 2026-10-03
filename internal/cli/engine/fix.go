// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/scandrix/backend/internal/pathguard"
	"github.com/scandrix/backend/pkg/models"
)

// BatchFixResult encapsulates outcomes of applying multiple remediations across files.
type BatchFixResult struct {
	Applied int      `json:"applied"`
	Failed  int      `json:"failed"`
	Details []string `json:"details"`
}

// CanApplyFix checks if a finding can be safely applied to disk.
func CanApplyFix(targetDir string, finding models.CodeFinding) bool {
	if finding.FilePath == "" || strings.TrimSpace(finding.SuggestedDiff) == "" {
		return false
	}
	if targetDir == "" {
		targetDir = "."
	}
	// finding.FilePath comes from review output, i.e. ultimately from a model,
	// so it is confined to the repository before being stat'd.
	fullPath, pathErr := pathguard.ResolvePath(targetDir, finding.FilePath)
	if pathErr != nil {
		return false
	}
	info, err := os.Stat(fullPath)
	return err == nil && !info.IsDir()
}

// GenerateDiffPreview constructs a readable diff snippet for a finding.
func GenerateDiffPreview(finding models.CodeFinding) string {
	if strings.TrimSpace(finding.SuggestedDiff) == "" {
		return "No automated remediation available"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s (lines %d-%d)\n", finding.FilePath, finding.StartLine, finding.EndLine))
	sb.WriteString("Suggested Patch:\n")
	for _, l := range strings.Split(finding.SuggestedDiff, "\n") {
		sb.WriteString("  " + l + "\n")
	}
	return sb.String()
}

// extractCleanReplacement parses unified diff markers (+/-) into clean source replacement lines.
func extractCleanReplacement(diff string) []string {
	replacement := strings.TrimSpace(diff)
	if strings.Contains(replacement, "@@") || strings.Contains(replacement, "\n+") || strings.Contains(replacement, "\n-") || strings.HasPrefix(replacement, "+") || strings.HasPrefix(replacement, "-") {
		var newLines []string
		for _, diffLine := range strings.Split(replacement, "\n") {
			if strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "+++") {
				newLines = append(newLines, strings.TrimPrefix(diffLine, "+"))
			} else if !strings.HasPrefix(diffLine, "-") && !strings.HasPrefix(diffLine, "@@") && !strings.HasPrefix(diffLine, "---") {
				newLines = append(newLines, diffLine)
			}
		}
		if len(newLines) > 0 {
			return newLines
		}
	}
	return strings.Split(replacement, "\n")
}

// ApplyBatchFixes groups findings by file, sorts by StartLine in descending order,
// and applies them from bottom to top so line index shifts do not corrupt earlier offsets.
func ApplyBatchFixes(targetDir string, findings []models.CodeFinding) (*BatchFixResult, error) {
	if targetDir == "" {
		targetDir = "."
	}

	res := &BatchFixResult{
		Details: make([]string, 0),
	}

	// 1. Group fixable findings by file path
	fileMap := make(map[string][]models.CodeFinding)
	for _, f := range findings {
		if f.FilePath == "" || strings.TrimSpace(f.SuggestedDiff) == "" {
			continue
		}
		fileMap[f.FilePath] = append(fileMap[f.FilePath], f)
	}

	if len(fileMap) == 0 {
		return res, nil
	}

	// 2. Process each file independently
	for relPath, list := range fileMap {
		fullPath, pathErr := pathguard.ResolvePath(targetDir, relPath)
		if pathErr != nil {
			res.Failed += len(list)
			res.Details = append(res.Details, fmt.Sprintf("Refusing %s: %v", relPath, pathErr))
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			res.Failed += len(list)
			res.Details = append(res.Details, fmt.Sprintf("Failed reading %s: %v", relPath, err))
			continue
		}

		info, statErr := os.Stat(fullPath)
		mode := os.FileMode(0644)
		if statErr == nil {
			mode = info.Mode()
		}

		lines := strings.Split(string(data), "\n")

		// Sort findings descending by StartLine
		sort.Slice(list, func(i, j int) bool {
			return list[i].StartLine > list[j].StartLine
		})

		fileApplied := 0
		for _, f := range list {
			start := f.StartLine - 1
			end := f.EndLine

			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
			if end < start {
				end = start
			}
			if end > len(lines) {
				end = len(lines)
			}

			replacementLines := extractCleanReplacement(f.SuggestedDiff)

			// Splice lines from bottom to top
			var modified []string
			modified = append(modified, lines[:start]...)
			modified = append(modified, replacementLines...)
			if end < len(lines) {
				modified = append(modified, lines[end:]...)
			}
			lines = modified
			fileApplied++
		}

		output := strings.Join(lines, "\n")
		if err := os.WriteFile(fullPath, []byte(output), mode); err != nil { // #nosec G703 -- fullPath comes from pathguard.ResolvePath against targetDir
			res.Failed += len(list)
			res.Details = append(res.Details, fmt.Sprintf("Failed writing %s: %v", relPath, err))
			continue
		}

		res.Applied += fileApplied
		res.Details = append(res.Details, fmt.Sprintf("Applied %d fixes to %s", fileApplied, relPath))
	}

	return res, nil
}

// ApplyFindingFix attempts to apply the suggested fix in a CodeFinding to the local file.
func ApplyFindingFix(targetDir string, finding models.CodeFinding) (string, error) {
	batchRes, err := ApplyBatchFixes(targetDir, []models.CodeFinding{finding})
	if err != nil {
		return "", err
	}
	if batchRes.Applied == 0 {
		if len(batchRes.Details) > 0 {
			return "", fmt.Errorf("%s", batchRes.Details[0])
		}
		return "", fmt.Errorf("no automated code patch available for this finding (manual remediation required)")
	}
	return fmt.Sprintf("Applied fix to %s (lines %d-%d)", finding.FilePath, finding.StartLine, finding.EndLine), nil
}
