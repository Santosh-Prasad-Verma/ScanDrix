// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package diff

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	extendedDiffHeaderRegex = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	extendedHunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)
	similarityRegex         = regexp.MustCompile(`^similarity index (\d+)%$`)
	oldModeRegex            = regexp.MustCompile(`^old mode (\d+)$`)
	newModeRegex            = regexp.MustCompile(`^new mode (\d+)$`)
	newFileModeRegex        = regexp.MustCompile(`^new file mode (\d+)$`)
	deletedFileModeRegex    = regexp.MustCompile(`^deleted file mode (\d+)$`)
	submoduleCommitRegex    = regexp.MustCompile(`^[+-]Subproject commit ([0-9a-fA-F]+)$`)
)

// ExtendedFilePatch models full git change metadata including modes, renames, copies, and submodules.
type ExtendedFilePatch struct {
	OldPath            string
	NewPath            string
	OldMode            string
	NewMode            string
	IsNew              bool
	IsDeleted          bool
	IsRename           bool
	RenameFrom         string
	RenameTo           string
	SimilarityIndex    int
	IsCopy             bool
	CopyFrom           string
	CopyTo             string
	IsSubmodule        bool
	SubmoduleOldCommit string
	SubmoduleNewCommit string
	IsBinary           bool
	Additions          int
	Deletions          int
	Hunks              []Hunk
}

// ParseExtendedUnifiedDiff parses multi-file diff streams with git extended metadata.
func ParseExtendedUnifiedDiff(r io.Reader) ([]*ExtendedFilePatch, error) {
	reader := bufio.NewReaderSize(r, 64*1024)

	var patches []*ExtendedFilePatch
	var currentPatch *ExtendedFilePatch
	var currentHunk *Hunk
	var oldLineTracker, newLineTracker int
	totalLines := 0
	totalHunks := 0

	for {
		lineBytes, isPrefix, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error reading extended diff stream: %w", err)
		}

		if isPrefix {
			for isPrefix && err == nil {
				_, isPrefix, err = reader.ReadLine()
			}
		}

		totalLines++
		if totalLines > MaxDiffLines {
			break
		}

		line := string(lineBytes)

		// Check for diff header
		if matches := extendedDiffHeaderRegex.FindStringSubmatch(line); len(matches) == 3 {
			if len(patches) >= MaxDiffFiles {
				break
			}
			if currentHunk != nil && currentPatch != nil {
				currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
				currentHunk = nil
			}
			if currentPatch != nil {
				patches = append(patches, currentPatch)
			}
			currentPatch = &ExtendedFilePatch{
				OldPath: matches[1],
				NewPath: matches[2],
			}
			continue
		}

		if currentPatch == nil {
			continue
		}

		// Extended headers
		if strings.HasPrefix(line, "similarity index") {
			if m := similarityRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.SimilarityIndex, _ = strconv.Atoi(m[1])
			}
			continue
		}
		if strings.HasPrefix(line, "rename from ") {
			currentPatch.IsRename = true
			currentPatch.RenameFrom = strings.TrimPrefix(line, "rename from ")
			continue
		}
		if strings.HasPrefix(line, "rename to ") {
			currentPatch.IsRename = true
			currentPatch.RenameTo = strings.TrimPrefix(line, "rename to ")
			continue
		}
		if strings.HasPrefix(line, "copy from ") {
			currentPatch.IsCopy = true
			currentPatch.CopyFrom = strings.TrimPrefix(line, "copy from ")
			continue
		}
		if strings.HasPrefix(line, "copy to ") {
			currentPatch.IsCopy = true
			currentPatch.CopyTo = strings.TrimPrefix(line, "copy to ")
			continue
		}
		if strings.HasPrefix(line, "old mode ") {
			if m := oldModeRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.OldMode = m[1]
			}
			continue
		}
		if strings.HasPrefix(line, "new mode ") {
			if m := newModeRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.NewMode = m[1]
			}
			continue
		}
		if strings.HasPrefix(line, "new file mode ") {
			currentPatch.IsNew = true
			if m := newFileModeRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.NewMode = m[1]
			}
			continue
		}
		if strings.HasPrefix(line, "deleted file mode ") {
			currentPatch.IsDeleted = true
			if m := deletedFileModeRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.OldMode = m[1]
			}
			continue
		}
		if strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch") {
			currentPatch.IsBinary = true
			continue
		}

		// Submodule changes
		if strings.HasPrefix(line, "-Subproject commit ") {
			currentPatch.IsSubmodule = true
			if m := submoduleCommitRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.SubmoduleOldCommit = m[1]
			}
			continue
		}
		if strings.HasPrefix(line, "+Subproject commit ") {
			currentPatch.IsSubmodule = true
			if m := submoduleCommitRegex.FindStringSubmatch(line); len(m) == 2 {
				currentPatch.SubmoduleNewCommit = m[1]
			}
			continue
		}

		// Hunk headers
		if matches := extendedHunkHeaderRegex.FindStringSubmatch(line); len(matches) >= 5 {
			if totalHunks >= MaxTotalHunks || len(currentPatch.Hunks) >= MaxHunksPerFile {
				continue
			}
			if currentHunk != nil {
				currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
			}
			totalHunks++

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

			headerComment := ""
			if len(matches) >= 6 {
				headerComment = strings.TrimSpace(matches[5])
			}

			currentHunk = &Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Header:   headerComment,
			}

			oldLineTracker = oldStart
			newLineTracker = newStart
			continue
		}

		if currentHunk == nil {
			continue
		}

		// Diff content lines
		if len(line) == 0 {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineContext,
				OldLineNo: oldLineTracker,
				NewLineNo: newLineTracker,
				Content:   "",
			})
			oldLineTracker++
			newLineTracker++
			continue
		}

		prefix := line[0]
		content := line[1:]

		switch prefix {
		case '+':
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineAddition,
				OldLineNo: 0,
				NewLineNo: newLineTracker,
				Content:   content,
			})
			newLineTracker++
			currentPatch.Additions++
		case '-':
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineDeletion,
				OldLineNo: oldLineTracker,
				NewLineNo: 0,
				Content:   content,
			})
			oldLineTracker++
			currentPatch.Deletions++
		case ' ':
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineContext,
				OldLineNo: oldLineTracker,
				NewLineNo: newLineTracker,
				Content:   content,
			})
			oldLineTracker++
			newLineTracker++
		}
	}

	if currentHunk != nil && currentPatch != nil {
		currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
	}
	if currentPatch != nil {
		patches = append(patches, currentPatch)
	}

	return patches, nil
}

// SCMCoordinate represents resolved line positioning for SCM Review Comment APIs.
type SCMCoordinate struct {
	DiffPosition  int    // 1-based line offset in the file's diff patch starting at 1 from first @@
	HunkIndex     int    // Index of the containing hunk
	Side          string // "RIGHT" for new/context lines, "LEFT" for deleted lines
	LineNumber    int    // The target line number in the source file
	IsValidTarget bool   // Whether the line falls within a modified hunk
}

// SCMCoordinateResolver maps source line numbers to SCM diff position coordinates.
type SCMCoordinateResolver struct{}

// NewSCMCoordinateResolver constructs a new coordinate resolver.
func NewSCMCoordinateResolver() *SCMCoordinateResolver {
	return &SCMCoordinateResolver{}
}

// ResolveLinePosition computes the exact 1-based diff position required by GitHub/GitLab APIs.
func (r *SCMCoordinateResolver) ResolveLinePosition(patch *ExtendedFilePatch, targetLine int, side string) SCMCoordinate {
	if patch == nil || len(patch.Hunks) == 0 || targetLine <= 0 {
		return SCMCoordinate{IsValidTarget: false}
	}

	normalizedSide := strings.ToUpper(strings.TrimSpace(side))
	if normalizedSide == "" {
		normalizedSide = "RIGHT"
	}

	currentDiffPosition := 0

	for hIdx, hunk := range patch.Hunks {
		currentDiffPosition++ // Hunk header line @@ ... @@ counts as 1 position

		for _, line := range hunk.Lines {
			currentDiffPosition++

			if normalizedSide == "RIGHT" {
				if (line.Type == LineAddition || line.Type == LineContext) && line.NewLineNo == targetLine {
					return SCMCoordinate{
						DiffPosition:  currentDiffPosition,
						HunkIndex:     hIdx,
						Side:          "RIGHT",
						LineNumber:    targetLine,
						IsValidTarget: true,
					}
				}
			} else if normalizedSide == "LEFT" {
				if (line.Type == LineDeletion || line.Type == LineContext) && line.OldLineNo == targetLine {
					return SCMCoordinate{
						DiffPosition:  currentDiffPosition,
						HunkIndex:     hIdx,
						Side:          "LEFT",
						LineNumber:    targetLine,
						IsValidTarget: true,
					}
				}
			}
		}
	}

	return SCMCoordinate{IsValidTarget: false}
}

// ValidateMultiLineRange verifies and clamps multi-line comment spans within a single hunk.
func (r *SCMCoordinateResolver) ValidateMultiLineRange(
	patch *ExtendedFilePatch,
	startLine, endLine int,
	side string,
) (clampedStart, clampedEnd int, ok bool) {
	if patch == nil || len(patch.Hunks) == 0 || startLine <= 0 || endLine <= 0 {
		return 0, 0, false
	}
	if endLine < startLine {
		startLine, endLine = endLine, startLine
	}

	startCoord := r.ResolveLinePosition(patch, startLine, side)
	endCoord := r.ResolveLinePosition(patch, endLine, side)

	// If both lines are valid and share the same hunk
	if startCoord.IsValidTarget && endCoord.IsValidTarget && startCoord.HunkIndex == endCoord.HunkIndex {
		return startLine, endLine, true
	}

	// Fallback: Check if startLine overlaps any hunk and clamp endLine to the hunk boundary
	for _, hunk := range patch.Hunks {
		var hunkStart, hunkEnd int
		if strings.ToUpper(side) == "LEFT" {
			hunkStart = hunk.OldStart
			hunkEnd = hunk.OldStart + hunk.OldLines - 1
		} else {
			hunkStart = hunk.NewStart
			hunkEnd = hunk.NewStart + hunk.NewLines - 1
		}

		if startLine >= hunkStart && startLine <= hunkEnd {
			cEnd := endLine
			if cEnd > hunkEnd {
				cEnd = hunkEnd
			}
			return startLine, cEnd, true
		}
	}

	return 0, 0, false
}

// DiffChunker partitions large diffs into token-budgeted review batches.
type DiffChunker struct{}

// NewDiffChunker constructs a new diff chunker.
func NewDiffChunker() *DiffChunker {
	return &DiffChunker{}
}

// EstimatePatchTokens calculates token consumption for an extended file patch.
func (c *DiffChunker) EstimatePatchTokens(patch *ExtendedFilePatch) int {
	if patch == nil {
		return 0
	}
	charCount := len(patch.NewPath) + len(patch.OldPath) + 128
	for _, h := range patch.Hunks {
		charCount += len(h.Header) + 32
		for _, l := range h.Lines {
			charCount += len(l.Content) + 2
		}
	}
	return (charCount + 3) / 4
}

// ChunkPatchesByTokenBudget groups files into batches under maxTokens with test/implementation co-location.
func (c *DiffChunker) ChunkPatchesByTokenBudget(
	patches []*ExtendedFilePatch,
	maxTokens int,
) [][]*ExtendedFilePatch {
	if len(patches) == 0 {
		return nil
	}
	if maxTokens <= 0 {
		return [][]*ExtendedFilePatch{patches}
	}

	// Group implementations with their tests if possible
	sortedPatches := make([]*ExtendedFilePatch, len(patches))
	copy(sortedPatches, patches)

	sort.SliceStable(sortedPatches, func(i, j int) bool {
		baseI := strings.TrimSuffix(filepath.Base(sortedPatches[i].NewPath), "_test.go")
		baseJ := strings.TrimSuffix(filepath.Base(sortedPatches[j].NewPath), "_test.go")
		if baseI == baseJ {
			return !strings.HasSuffix(sortedPatches[i].NewPath, "_test.go")
		}
		return sortedPatches[i].NewPath < sortedPatches[j].NewPath
	})

	var batches [][]*ExtendedFilePatch
	var currentBatch []*ExtendedFilePatch
	currentTokens := 0

	for _, p := range sortedPatches {
		tokens := c.EstimatePatchTokens(p)

		if tokens >= maxTokens {
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = nil
				currentTokens = 0
			}
			batches = append(batches, []*ExtendedFilePatch{p})
			continue
		}

		if currentTokens+tokens > maxTokens && len(currentBatch) > 0 {
			batches = append(batches, currentBatch)
			currentBatch = []*ExtendedFilePatch{p}
			currentTokens = tokens
		} else {
			currentBatch = append(currentBatch, p)
			currentTokens += tokens
		}
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// DiffSynthesizer reconstructs standard unified diff text from structured patches.
type DiffSynthesizer struct{}

// NewDiffSynthesizer constructs a patch synthesizer.
func NewDiffSynthesizer() *DiffSynthesizer {
	return &DiffSynthesizer{}
}

// SynthesizePatch formats an ExtendedFilePatch into standard git patch text.
func (s *DiffSynthesizer) SynthesizePatch(patch *ExtendedFilePatch) string {
	if patch == nil {
		return ""
	}

	var sb strings.Builder
	oldP := patch.OldPath
	if oldP == "" {
		oldP = patch.NewPath
	}
	newP := patch.NewPath
	if newP == "" {
		newP = patch.OldPath
	}

	sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", oldP, newP))

	if patch.OldMode != "" && patch.NewMode != "" && patch.OldMode != patch.NewMode {
		sb.WriteString(fmt.Sprintf("old mode %s\n", patch.OldMode))
		sb.WriteString(fmt.Sprintf("new mode %s\n", patch.NewMode))
	}
	if patch.IsNew {
		sb.WriteString(fmt.Sprintf("new file mode %s\n", patch.NewMode))
	}
	if patch.IsDeleted {
		sb.WriteString(fmt.Sprintf("deleted file mode %s\n", patch.OldMode))
	}
	if patch.IsRename {
		sb.WriteString(fmt.Sprintf("similarity index %d%%\n", patch.SimilarityIndex))
		sb.WriteString(fmt.Sprintf("rename from %s\n", patch.RenameFrom))
		sb.WriteString(fmt.Sprintf("rename to %s\n", patch.RenameTo))
	}
	if patch.IsCopy {
		sb.WriteString(fmt.Sprintf("similarity index %d%%\n", patch.SimilarityIndex))
		sb.WriteString(fmt.Sprintf("copy from %s\n", patch.CopyFrom))
		sb.WriteString(fmt.Sprintf("copy to %s\n", patch.CopyTo))
	}
	if patch.IsBinary {
		sb.WriteString(fmt.Sprintf("Binary files a/%s and b/%s differ\n", oldP, newP))
		return sb.String()
	}

	if patch.IsNew {
		sb.WriteString("--- /dev/null\n")
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", newP))
	} else if patch.IsDeleted {
		sb.WriteString(fmt.Sprintf("--- a/%s\n", oldP))
		sb.WriteString("+++ /dev/null\n")
	} else {
		sb.WriteString(fmt.Sprintf("--- a/%s\n", oldP))
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", newP))
	}

	for _, h := range patch.Hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines))
		if h.Header != "" {
			sb.WriteString(" " + h.Header)
		}
		sb.WriteString("\n")

		for _, l := range h.Lines {
			switch l.Type {
			case LineAddition:
				sb.WriteString("+" + l.Content + "\n")
			case LineDeletion:
				sb.WriteString("-" + l.Content + "\n")
			case LineContext:
				sb.WriteString(" " + l.Content + "\n")
			}
		}
	}

	return sb.String()
}
