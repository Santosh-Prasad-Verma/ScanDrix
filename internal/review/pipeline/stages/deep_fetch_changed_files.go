package stages

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
)

const (
	LegacyMaxFiles = 350
	AgentMaxFiles  = 2000
)

var hunkHeaderRegex = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ExtractValidDiffLines extracts valid line ranges from a unified diff patch.
// Returns an array of [start, end] tuples representing lines on the RIGHT side
// that SCM providers (GitHub, GitLab, Bitbucket) permit for inline comments.
func ExtractValidDiffLines(patch string) [][2]int {
	if strings.TrimSpace(patch) == "" {
		return nil
	}

	var ranges [][2]int
	lines := strings.Split(patch, "\n")
	rightLine := 0
	hunkStart := 0

	for _, line := range lines {
		// Match hunk header @@ -oldStart,oldCount +newStart,newCount @@
		matches := hunkHeaderRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			// Save previous hunk range if non-empty
			if hunkStart > 0 && rightLine > hunkStart {
				ranges = append(ranges, [2]int{hunkStart, rightLine - 1})
			}
			startNum, err := strconv.Atoi(matches[1])
			if err == nil {
				rightLine = startNum
				hunkStart = rightLine
			}
			continue
		}

		if hunkStart == 0 {
			continue // Before first hunk header
		}

		if strings.HasPrefix(line, "-") {
			// Deleted line: only exists on LEFT side, does not advance right-side line counter
			continue
		}

		if strings.HasPrefix(line, `\`) {
			// "No newline at end of file" warning, skip
			continue
		}

		// Context line (space prefix) or added line (+): exists on RIGHT side
		rightLine++
	}

	// Save final hunk range
	if hunkStart > 0 && rightLine > hunkStart {
		ranges = append(ranges, [2]int{hunkStart, rightLine - 1})
	}

	return ranges
}

// ConvertToUnifiedDiffWithLineNumbers decorates patch additions and context with right-side line numbers.
func ConvertToUnifiedDiffWithLineNumbers(patch, filename string) string {
	if strings.TrimSpace(patch) == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", filename, filename))

	lines := strings.Split(patch, "\n")
	rightLine := 0
	leftLine := 0

	for _, line := range lines {
		matches := hunkHeaderRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			sb.WriteString(line)
			sb.WriteString("\n")
			startNum, _ := strconv.Atoi(matches[1])
			rightLine = startNum
			continue
		}

		if strings.HasPrefix(line, "+") {
			sb.WriteString(fmt.Sprintf("+ %4d | %s\n", rightLine, line[1:]))
			rightLine++
		} else if strings.HasPrefix(line, "-") {
			sb.WriteString(fmt.Sprintf("-      | %s\n", line[1:]))
			leftLine++
		} else if strings.HasPrefix(line, " ") {
			sb.WriteString(fmt.Sprintf("  %4d | %s\n", rightLine, line[1:]))
			rightLine++
			leftLine++
		} else {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// DeepFetchChangedFilesStage implements Stage 5 changed files ingestion, glob path filtering, diff line extraction, and PR stats calculation.
type DeepFetchChangedFilesStage struct {
	maxFiles int
}

// NewDeepFetchChangedFilesStage constructs Stage 5.
func NewDeepFetchChangedFilesStage(maxFiles int) *DeepFetchChangedFilesStage {
	if maxFiles <= 0 {
		maxFiles = AgentMaxFiles
	}
	return &DeepFetchChangedFilesStage{maxFiles: maxFiles}
}

func (s *DeepFetchChangedFilesStage) Name() string {
	return "DeepFetchChangedFilesStage"
}

func (s *DeepFetchChangedFilesStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	// 1. Ensure raw diff is parsed into patches if not already present
	if len(pCtx.ParsedPatches) == 0 && strings.TrimSpace(pCtx.RawDiff) != "" {
		patches, err := diff.ParseUnifiedDiff(bufio.NewReader(strings.NewReader(pCtx.RawDiff)))
		if err == nil {
			pCtx.ParsedPatches = patches
		}
	}

	if len(pCtx.ParsedPatches) == 0 {
		pCtx.SkipReview = true
		pCtx.SkipReason = "Pull request contains 0 file changes"
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    "No changed files detected in pull request",
			ReasonCode: "NO_FILES_IN_PR",
		}
		return nil
	}

	ignorePaths := pCtx.ResolvedConfig.IgnorePaths

	var toAnalyze []pipeline.FileChangeInfo
	var ignoredFiles []string
	var filteredPatches []*diff.FilePatch

	totalAdditions := 0
	totalDeletions := 0

	for _, patch := range pCtx.ParsedPatches {
		filename := patch.NewPath
		if filename == "" {
			filename = patch.OldPath
		}

		status := "modified"
		if patch.IsNew {
			status = "added"
		} else if patch.IsDeleted {
			status = "removed"
		}

		// Reconstruct hunk patch text
		var patchBuilder strings.Builder
		for _, h := range patch.Hunks {
			patchBuilder.WriteString(h.Header)
			patchBuilder.WriteString("\n")
			for _, l := range h.Lines {
				prefix := " "
				if l.Type == diff.LineAddition {
					prefix = "+"
				} else if l.Type == diff.LineDeletion {
					prefix = "-"
				}
				patchBuilder.WriteString(prefix)
				patchBuilder.WriteString(l.Content)
				patchBuilder.WriteString("\n")
			}
		}
		patchText := patchBuilder.String()

		// Filter out deleted files or files matching ignore paths
		if patch.IsDeleted || pCtx.ResolvedConfig.MatchesPathPatterns(filename, ignorePaths) {
			ignoredFiles = append(ignoredFiles, filename)
			continue
		}

		validLines := ExtractValidDiffLines(patchText)
		patchWithLineNums := ConvertToUnifiedDiffWithLineNumbers(patchText, filename)

		fileInfo := pipeline.FileChangeInfo{
			Filename:          filename,
			OldPath:           patch.OldPath,
			Status:            status,
			Additions:         patch.Additions,
			Deletions:         patch.Deletions,
			Patch:             patchText,
			PatchWithLinesStr: patchWithLineNums,
			ValidDiffLines:    validLines,
		}

		toAnalyze = append(toAnalyze, fileInfo)
		filteredPatches = append(filteredPatches, patch)

		totalAdditions += patch.Additions
		totalDeletions += patch.Deletions
	}

	pCtx.IgnoredFiles = ignoredFiles

	// 2. Validate resulting files count
	if len(toAnalyze) == 0 {
		pCtx.SkipReview = true
		sampleIgnored := ignoredFiles
		if len(sampleIgnored) > 5 {
			sampleIgnored = sampleIgnored[:5]
		}
		reason := fmt.Sprintf("All %d changed files were ignored by configured paths (e.g. %s)", len(pCtx.ParsedPatches), strings.Join(sampleIgnored, ", "))
		pCtx.SkipReason = reason
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    reason,
			ReasonCode: "ALL_FILES_IGNORED",
		}
		return nil
	}

	if len(toAnalyze) > s.maxFiles {
		pCtx.SkipReview = true
		reason := fmt.Sprintf("Changed files count (%d) exceeds limit of %d files per review", len(toAnalyze), s.maxFiles)
		pCtx.SkipReason = reason
		pCtx.StatusInfo = pipeline.PipelineStatusInfo{
			Status:     pipeline.StatusSkipped,
			Message:    reason,
			ReasonCode: "TOO_MANY_FILES",
		}
		return nil
	}

	// 3. Populate context
	pCtx.ChangedFiles = toAnalyze
	pCtx.FilteredPatches = filteredPatches
	pCtx.PRStats = pipeline.PRStatsInfo{
		TotalAdditions:    totalAdditions,
		TotalDeletions:    totalDeletions,
		TotalFiles:        len(toAnalyze),
		TotalLinesChanged: totalAdditions + totalDeletions,
	}

	return nil
}
