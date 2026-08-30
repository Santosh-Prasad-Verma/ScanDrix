package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// ApplyFindingFix attempts to apply the suggested fix in a CodeFinding to the local file.
func ApplyFindingFix(finding models.CodeFinding) (string, error) {
	if finding.FilePath == "" {
		return "", fmt.Errorf("finding has no target file path")
	}

	if finding.SuggestedDiff == "" && finding.Remediation == "" {
		return "", fmt.Errorf("no automated fix available for this finding")
	}

	// 1. Read existing file
	data, err := os.ReadFile(finding.FilePath)
	if err != nil {
		return "", fmt.Errorf("failed reading %s: %w", finding.FilePath, err)
	}

	lines := strings.Split(string(data), "\n")
	start := finding.StartLine - 1
	end := finding.EndLine

	if start < 0 {
		start = 0
	}
	if start > len(lines) {
		start = len(lines)
	}
	if end > len(lines) {
		end = len(lines)
	}

	// 2. If SuggestedDiff contains unified diff syntax, extract added lines
	replacement := finding.SuggestedDiff
	if strings.Contains(replacement, "@@") || strings.Contains(replacement, "+") || strings.Contains(replacement, "-") {
		var newLines []string
		for _, diffLine := range strings.Split(replacement, "\n") {
			if strings.HasPrefix(diffLine, "+") && !strings.HasPrefix(diffLine, "+++") {
				newLines = append(newLines, strings.TrimPrefix(diffLine, "+"))
			} else if !strings.HasPrefix(diffLine, "-") && !strings.HasPrefix(diffLine, "@@") && !strings.HasPrefix(diffLine, "---") {
				newLines = append(newLines, diffLine)
			}
		}
		if len(newLines) > 0 {
			replacement = strings.Join(newLines, "\n")
		}
	}

	replacementLines := strings.Split(replacement, "\n")

	// 3. Splice lines
	var modified []string
	modified = append(modified, lines[:start]...)
	modified = append(modified, replacementLines...)
	if end < len(lines) {
		modified = append(modified, lines[end:]...)
	}

	// 4. Write back
	output := strings.Join(modified, "\n")
	if err := os.WriteFile(finding.FilePath, []byte(output), 0644); err != nil {
		return "", fmt.Errorf("failed writing fix to %s: %w", finding.FilePath, err)
	}

	return fmt.Sprintf("Applied fix to %s (lines %d-%d)", finding.FilePath, finding.StartLine, finding.EndLine), nil
}
