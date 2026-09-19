// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	// hunkHeaderRegex matches unified diff hunk headers: @@ -oldStart,oldCount +newStart,newCount @@
	hunkHeaderRegex = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
)

// LineRange represents an inclusive [Start, End] range of lines in a file.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// DiffBoundaryValidator validates line numbers against unified diff patches
// to guarantee that inline review comments are only placed on lines present
// in the diff (added or context lines), avoiding SCM platform 422 Unprocessable Entity errors.
type DiffBoundaryValidator struct{}

// NewDiffBoundaryValidator constructs a new boundary validator.
func NewDiffBoundaryValidator() *DiffBoundaryValidator {
	return &DiffBoundaryValidator{}
}

// ExtractValidDiffLines parses a unified diff patch string and returns all valid
// line ranges on the right-hand (new/modified) side of the diff.
func (v *DiffBoundaryValidator) ExtractValidDiffLines(patch string) []LineRange {
	if strings.TrimSpace(patch) == "" {
		return nil
	}

	lines := strings.Split(patch, "\n")
	var ranges []LineRange

	var rightLine int
	var currentHunkStart int
	inHunk := false

	for _, line := range lines {
		matches := hunkHeaderRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			// If we were already tracking a hunk, close it
			if inHunk && rightLine >= currentHunkStart && currentHunkStart > 0 {
				ranges = append(ranges, LineRange{
					Start: currentHunkStart,
					End:   rightLine,
				})
			}

			startVal, err := strconv.Atoi(matches[1])
			if err != nil {
				inHunk = false
				continue
			}

			rightLine = startVal
			currentHunkStart = startVal
			inHunk = true
			continue
		}

		if !inHunk {
			continue
		}

		if len(line) == 0 {
			// Empty line inside hunk treated as context line
			rightLine++
			continue
		}

		prefix := line[0]
		switch prefix {
		case '+':
			// Added line increments rightLine
			rightLine++
		case ' ':
			// Context line increments rightLine
			rightLine++
		case '-':
			// Deleted line only exists on old side; does not increment rightLine
			continue
		case '\\':
			// "\ No newline at end of file"
			continue
		default:
			// Treat unexpected lines as context to prevent premature cutoff
			rightLine++
		}
	}

	if inHunk && rightLine > currentHunkStart && currentHunkStart > 0 {
		ranges = append(ranges, LineRange{
			Start: currentHunkStart,
			End:   rightLine - 1,
		})
	}

	return collapseRanges(ranges)
}

// collapseRanges merges overlapping or directly contiguous line ranges.
func collapseRanges(ranges []LineRange) []LineRange {
	if len(ranges) <= 1 {
		return ranges
	}

	var merged []LineRange
	current := ranges[0]

	for i := 1; i < len(ranges); i++ {
		next := ranges[i]
		if next.Start <= current.End+1 {
			if next.End > current.End {
				current.End = next.End
			}
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)
	return merged
}

// IsLineInDiff checks whether a target line number falls within any valid diff range.
func (v *DiffBoundaryValidator) IsLineInDiff(patch string, line int) bool {
	ranges := v.ExtractValidDiffLines(patch)
	for _, r := range ranges {
		if line >= r.Start && line <= r.End {
			return true
		}
	}
	return false
}

// ClipToValidRange attempts to adjust [start, end] so it fits within the closest
// valid diff hunk, or returns false if no overlapping or adjacent hunk exists.
func (v *DiffBoundaryValidator) ClipToValidRange(patch string, start, end int) (int, int, bool) {
	ranges := v.ExtractValidDiffLines(patch)
	if len(ranges) == 0 {
		return start, end, false
	}

	for _, r := range ranges {
		// Complete overlap
		if start >= r.Start && end <= r.End {
			return start, end, true
		}

		// Partial overlap
		if start <= r.End && end >= r.Start {
			clippedStart := max(start, r.Start)
			clippedEnd := min(end, r.End)
			return clippedStart, clippedEnd, true
		}
	}

	// If the line falls within 3 lines of a diff hunk, snap to that hunk boundary
	for _, r := range ranges {
		if start >= r.Start-3 && start <= r.End+3 {
			clippedStart := max(start, r.Start)
			clippedEnd := min(end, r.End)
			if clippedStart <= clippedEnd {
				return clippedStart, clippedEnd, true
			}
			return r.Start, r.Start, true
		}
	}

	return start, end, false
}

// FilterFindings validates findings against the changed files list, adjusting line ranges
// or discarding findings that cannot be associated with any diff hunk.
func (v *DiffBoundaryValidator) FilterFindings(findings []AgentFinding, changedFiles []ChangedFile) ([]AgentFinding, []AgentFinding) {
	patchMap := make(map[string]string, len(changedFiles))
	for _, f := range changedFiles {
		patchMap[f.Filename] = f.Patch
	}

	var accepted []AgentFinding
	var discarded []AgentFinding

	for _, finding := range findings {
		patch, exists := patchMap[finding.FilePath]
		if !exists || strings.TrimSpace(patch) == "" {
			// PR-level or file without patch
			accepted = append(accepted, finding)
			continue
		}

		clippedStart, clippedEnd, ok := v.ClipToValidRange(patch, finding.StartLine, finding.EndLine)
		if ok {
			finding.StartLine = clippedStart
			finding.EndLine = clippedEnd
			accepted = append(accepted, finding)
		} else {
			discarded = append(discarded, finding)
		}
	}

	return accepted, discarded
}
