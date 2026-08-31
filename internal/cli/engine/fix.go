// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scandrix/backend/pkg/models"
)

// ApplyFindingFix attempts to apply the suggested fix in a CodeFinding to the local file.
func ApplyFindingFix(targetDir string, finding models.CodeFinding) (string, error) {
	if finding.FilePath == "" {
		return "", fmt.Errorf("finding has no target file path")
	}

	if finding.SuggestedDiff == "" && finding.Remediation == "" {
		return "", fmt.Errorf("no automated fix available for this finding")
	}

	fullPath := finding.FilePath
	if !filepath.IsAbs(fullPath) {
		if targetDir == "" {
			targetDir = "."
		}
		fullPath = filepath.Join(targetDir, fullPath)
	}

	// 1. Read existing file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed reading %s: %w", fullPath, err)
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
	if end < start {
		end = start
	}
	if end > len(lines) {
		end = len(lines)
	}

	// 2. If SuggestedDiff contains unified diff syntax, extract added lines
	replacement := finding.SuggestedDiff
	if replacement == "" {
		replacement = finding.Remediation
	}

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

	// 4. Write back atomically to preserve permissions
	info, statErr := os.Stat(fullPath)
	mode := os.FileMode(0644)
	if statErr == nil {
		mode = info.Mode()
	}

	output := strings.Join(modified, "\n")
	if err := os.WriteFile(fullPath, []byte(output), mode); err != nil {
		return "", fmt.Errorf("failed writing fix to %s: %w", fullPath, err)
	}

	return fmt.Sprintf("Applied fix to %s (lines %d-%d)", finding.FilePath, finding.StartLine, finding.EndLine), nil
}
