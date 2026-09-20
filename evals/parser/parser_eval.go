// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedHunk represents extracted diff coordinates from a unified diff hunk header.
type ParsedHunk struct {
	OldStart int `json:"old_start"`
	OldLines int `json:"old_lines"`
	NewStart int `json:"new_start"`
	NewLines int `json:"new_lines"`
	Header   string `json:"header"`
}

// DiffFile represents a parsed file within a unified diff.
type DiffFile struct {
	OldPath   string       `json:"old_path"`
	NewPath   string       `json:"new_path"`
	Hunks     []ParsedHunk `json:"hunks"`
	Additions int          `json:"additions"`
	Deletions int          `json:"deletions"`
}

var hunkRegex = regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)

// ParseUnifiedDiff parses unified git diff text into structured DiffFile slices.
func ParseUnifiedDiff(rawDiff string) ([]DiffFile, error) {
	if strings.TrimSpace(rawDiff) == "" {
		return nil, nil
	}

	lines := strings.Split(rawDiff, "\n")
	files := make([]DiffFile, 0)

	var currentFile *DiffFile
	var currentHunk *ParsedHunk

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if currentFile != nil {
				if currentHunk != nil {
					currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
					currentHunk = nil
				}
				files = append(files, *currentFile)
			}
			currentFile = &DiffFile{
				Hunks: make([]ParsedHunk, 0),
			}
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentFile.OldPath = strings.TrimPrefix(parts[2], "a/")
				currentFile.NewPath = strings.TrimPrefix(parts[3], "b/")
			}
			continue
		}

		if strings.HasPrefix(line, "--- ") && currentFile != nil {
			p := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			currentFile.OldPath = strings.TrimPrefix(p, "a/")
			continue
		}

		if strings.HasPrefix(line, "+++ ") && currentFile != nil {
			p := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			currentFile.NewPath = strings.TrimPrefix(p, "b/")
			continue
		}

		if strings.HasPrefix(line, "@@ ") && currentFile != nil {
			if currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			matches := hunkRegex.FindStringSubmatch(line)
			if len(matches) >= 4 {
				oldStart, _ := strconv.Atoi(matches[1])
				oldLines := 1
				if matches[2] != "" {
					oldLines, _ = strconv.Atoi(matches[2])
				}

				newStart, _ := strconv.Atoi(matches[3])
				newLines := 1
				if matches[4] != "" {
					newLines, _ = strconv.Atoi(matches[4])
				}

				header := ""
				if len(matches) > 5 {
					header = strings.TrimSpace(matches[5])
				}

				currentHunk = &ParsedHunk{
					OldStart: oldStart,
					OldLines: oldLines,
					NewStart: newStart,
					NewLines: newLines,
					Header:   header,
				}
			}
			continue
		}

		if currentFile != nil {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				currentFile.Additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				currentFile.Deletions++
			}
		}
	}

	if currentFile != nil {
		if currentHunk != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		}
		files = append(files, *currentFile)
	}

	return files, nil
}

// EvaluateParser asserts that a raw diff was correctly parsed into expected file paths and hunk counts.
func EvaluateParser(rawDiff string, expectedFiles int, expectedHunks int) (bool, string) {
	files, err := ParseUnifiedDiff(rawDiff)
	if err != nil {
		return false, fmt.Sprintf("Parser failed: %v", err)
	}

	if len(files) != expectedFiles {
		return false, fmt.Sprintf("Expected %d files, got %d", expectedFiles, len(files))
	}

	totalHunks := 0
	for _, f := range files {
		totalHunks += len(f.Hunks)
	}

	if totalHunks != expectedHunks {
		return false, fmt.Sprintf("Expected %d hunks, got %d", expectedHunks, totalHunks)
	}

	return true, "Passed"
}
