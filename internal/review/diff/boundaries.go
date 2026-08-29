package diff

// LineInterval represents an inclusive start and end line range.
type LineInterval struct {
	StartLine int
	EndLine   int
}

// ExtractHunkLineBoundaries returns all right-side (new file) line ranges present in the diff hunks.
// SCM providers (GitHub, GitLab) reject review comments with 422 if positioned outside these ranges.
func ExtractHunkLineBoundaries(patch *FilePatch) []LineInterval {
	if patch == nil || len(patch.Hunks) == 0 {
		return nil
	}

	var intervals []LineInterval
	for _, hunk := range patch.Hunks {
		hunkEnd := hunk.NewStart + hunk.NewLines - 1
		if hunk.NewLines <= 0 {
			hunkEnd = hunk.NewStart
		}
		intervals = append(intervals, LineInterval{
			StartLine: hunk.NewStart,
			EndLine:   hunkEnd,
		})
	}
	return intervals
}

// SnapFindingCoordinates aligns an AI-reported line span onto the nearest overlapping modified hunk.
// If the proposed finding does not overlap any modified hunk in the diff, it returns ok=false,
// signaling that the finding pertains to unchanged legacy code and should be suppressed to prevent SCM 422 errors.
func SnapFindingCoordinates(startLine, endLine int, intervals []LineInterval) (snappedStart int, snappedEnd int, ok bool) {
	if len(intervals) == 0 {
		return startLine, endLine, true
	}

	if startLine <= 0 && endLine <= 0 {
		return intervals[0].StartLine, intervals[0].EndLine, true
	}
	if startLine <= 0 {
		startLine = endLine
	}
	if endLine < startLine {
		endLine = startLine
	}

	var bestInterval *LineInterval
	maxOverlap := 0

	for _, inv := range intervals {
		// Check overlap: startLine <= inv.EndLine && endLine >= inv.StartLine
		if startLine <= inv.EndLine && endLine >= inv.StartLine {
			overlapStart := max(startLine, inv.StartLine)
			overlapEnd := min(endLine, inv.EndLine)
			overlap := overlapEnd - overlapStart + 1
			if overlap > maxOverlap {
				maxOverlap = overlap
				bestInterval = &LineInterval{
					StartLine: overlapStart,
					EndLine:   overlapEnd,
				}
			}
		}
	}

	if bestInterval != nil {
		return bestInterval.StartLine, bestInterval.EndLine, true
	}

	// No overlap with any changed line in the PR diff
	return 0, 0, false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
