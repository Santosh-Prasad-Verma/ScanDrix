// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package quickfix

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PatchHunk represents a single unified diff hunk.
type PatchHunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []string
}

// FilePatch represents all hunks targeting a specific file.
type FilePatch struct {
	OldPath string
	NewPath string
	Hunks   []PatchHunk
}

var hunkHeaderRegex = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// ParseUnifiedDiff parses a unified diff string into FilePatch structures.
func ParseUnifiedDiff(diffText string) ([]FilePatch, error) {
	lines := strings.Split(diffText, "\n")
	var patches []FilePatch
	var currentFile *FilePatch
	var currentHunk *PatchHunk

	for _, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			// Old file header
			if currentHunk != nil && currentFile != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				currentHunk = nil
			}
			if currentFile != nil {
				patches = append(patches, *currentFile)
			}
			oldPath := strings.TrimPrefix(line, "--- ")
			oldPath = strings.TrimPrefix(oldPath, "a/")
			currentFile = &FilePatch{OldPath: strings.TrimSpace(oldPath)}
			continue
		}

		if strings.HasPrefix(line, "+++ ") {
			if currentFile != nil {
				newPath := strings.TrimPrefix(line, "+++ ")
				newPath = strings.TrimPrefix(newPath, "b/")
				currentFile.NewPath = strings.TrimSpace(newPath)
			}
			continue
		}

		if strings.HasPrefix(line, "@@") {
			// Hunk header
			if currentHunk != nil && currentFile != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			matches := hunkHeaderRegex.FindStringSubmatch(line)
			if len(matches) < 4 {
				continue
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}

			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if len(matches) > 4 && matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			if currentFile == nil {
				currentFile = &FilePatch{OldPath: "target", NewPath: "target"}
			}

			currentHunk = &PatchHunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Lines:    []string{},
			}
			continue
		}

		if currentHunk != nil {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
				currentHunk.Lines = append(currentHunk.Lines, line)
			}
		}
	}

	if currentHunk != nil && currentFile != nil {
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
	}
	if currentFile != nil {
		patches = append(patches, *currentFile)
	}

	return patches, nil
}

// ApplyHunkToLines applies a single hunk to the provided source lines, using context matching.
func ApplyHunkToLines(original []string, hunk PatchHunk) ([]string, error) {
	// Extract expected context and old lines from hunk
	var expectedOld []string
	var newReplacement []string

	for _, l := range hunk.Lines {
		if strings.HasPrefix(l, " ") {
			content := strings.TrimPrefix(l, " ")
			expectedOld = append(expectedOld, content)
			newReplacement = append(newReplacement, content)
		} else if strings.HasPrefix(l, "-") {
			content := strings.TrimPrefix(l, "-")
			expectedOld = append(expectedOld, content)
		} else if strings.HasPrefix(l, "+") {
			content := strings.TrimPrefix(l, "+")
			newReplacement = append(newReplacement, content)
		}
	}

	if len(expectedOld) == 0 && len(newReplacement) > 0 {
		// Pure insertion at start line
		insertIdx := hunk.OldStart - 1
		if insertIdx < 0 {
			insertIdx = 0
		}
		if insertIdx > len(original) {
			insertIdx = len(original)
		}
		res := make([]string, 0, len(original)+len(newReplacement))
		res = append(res, original[:insertIdx]...)
		res = append(res, newReplacement...)
		res = append(res, original[insertIdx:]...)
		return res, nil
	}

	// Locate match index in original lines (try exact hunk.OldStart first, then fuzzy search)
	matchIdx := findMatchingPosition(original, expectedOld, hunk.OldStart-1)
	if matchIdx < 0 {
		return nil, fmt.Errorf("could not locate hunk context lines in target file")
	}

	res := make([]string, 0, len(original)-len(expectedOld)+len(newReplacement))
	res = append(res, original[:matchIdx]...)
	res = append(res, newReplacement...)
	endIdx := matchIdx + len(expectedOld)
	if endIdx < len(original) {
		res = append(res, original[endIdx:]...)
	}

	return res, nil
}

func findMatchingPosition(haystack, needle []string, hintIndex int) int {
	if len(needle) == 0 {
		return hintIndex
	}

	// 1. Try exact hintIndex
	if hintIndex >= 0 && hintIndex+len(needle) <= len(haystack) {
		matched := true
		for i := 0; i < len(needle); i++ {
			if strings.TrimSpace(haystack[hintIndex+i]) != strings.TrimSpace(needle[i]) {
				matched = false
				break
			}
		}
		if matched {
			return hintIndex
		}
	}

	// 2. Search entire file for matching block
	for i := 0; i <= len(haystack)-len(needle); i++ {
		matched := true
		for j := 0; j < len(needle); j++ {
			if strings.TrimSpace(haystack[i+j]) != strings.TrimSpace(needle[j]) {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}

	return -1
}
