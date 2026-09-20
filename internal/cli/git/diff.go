package git

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/scandrix/backend/internal/cli/types"
)

// DiffStats holds counts of additions and deletions.
type DiffStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
	Files     int `json:"files"`
}

// DiffHunk represents a unified diff hunk header and lines.
type DiffHunk struct {
	Header    string   `json:"header"`
	OldStart  int      `json:"old_start"`
	OldLines  int      `json:"old_lines"`
	NewStart  int      `json:"new_start"`
	NewLines  int      `json:"new_lines"`
	Heading   string   `json:"heading,omitempty"` // Function/class context
	Lines     []string `json:"lines"`
	Additions int      `json:"additions"`
	Deletions int      `json:"deletions"`
}

// ParsedFileDiff holds parsed diff data for a single file.
type ParsedFileDiff struct {
	OldPath   string     `json:"old_path"`
	NewPath   string     `json:"new_path"`
	Status    string     `json:"status"` // added, modified, deleted, renamed
	IsBinary  bool       `json:"is_binary"`
	Hunks     []DiffHunk `json:"hunks"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
}

var (
	hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)
	diffGitRegex    = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
)

// CountDiffChanges counts total additions, deletions, and touched files in a unified diff string.
func CountDiffChanges(diffText string) DiffStats {
	stats := DiffStats{}
	scanner := bufio.NewScanner(strings.NewReader(diffText))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			stats.Files++
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.Additions++
			stats.Total++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.Deletions++
			stats.Total++
		}
	}

	return stats
}

// ParseUnifiedDiff parses a multi-file git diff output into structured ParsedFileDiff slices.
func ParseUnifiedDiff(rawDiff string) ([]ParsedFileDiff, error) {
	var files []ParsedFileDiff
	scanner := bufio.NewScanner(strings.NewReader(rawDiff))

	var currentFile *ParsedFileDiff
	var currentHunk *DiffHunk

	flushHunk := func() {
		if currentFile != nil && currentHunk != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			currentHunk = nil
		}
	}

	flushFile := func() {
		flushHunk()
		if currentFile != nil {
			files = append(files, *currentFile)
			currentFile = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		if matches := diffGitRegex.FindStringSubmatch(line); len(matches) == 3 {
			flushFile()
			currentFile = &ParsedFileDiff{
				OldPath: matches[1],
				NewPath: matches[2],
				Status:  "modified",
			}
			continue
		}

		if currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "new file mode ") {
			currentFile.Status = "added"
			continue
		}
		if strings.HasPrefix(line, "deleted file mode ") {
			currentFile.Status = "deleted"
			continue
		}
		if strings.HasPrefix(line, "similarity index ") || strings.HasPrefix(line, "rename from ") {
			currentFile.Status = "renamed"
			continue
		}
		if strings.HasPrefix(line, "Binary files ") {
			currentFile.IsBinary = true
			continue
		}

		if strings.HasPrefix(line, "@@") {
			flushHunk()
			matches := hunkHeaderRegex.FindStringSubmatch(line)
			if len(matches) >= 6 {
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
				heading := strings.TrimSpace(matches[5])

				currentHunk = &DiffHunk{
					Header:   line,
					OldStart: oldStart,
					OldLines: oldLines,
					NewStart: newStart,
					NewLines: newLines,
					Heading:  heading,
					Lines:    make([]string, 0),
				}
			}
			continue
		}

		if currentHunk != nil {
			currentHunk.Lines = append(currentHunk.Lines, line)
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				currentHunk.Additions++
				currentFile.Additions++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				currentHunk.Deletions++
				currentFile.Deletions++
			}
		}
	}

	flushFile()
	return files, scanner.Err()
}

// ToTypesFileDiffs converts ParsedFileDiff slice into types.FileDiff slice.
func ToTypesFileDiffs(parsed []ParsedFileDiff) []types.FileDiff {
	res := make([]types.FileDiff, len(parsed))
	for i, p := range parsed {
		var patchBuilder strings.Builder
		for _, h := range p.Hunks {
			patchBuilder.WriteString(h.Header + "\n")
			for _, l := range h.Lines {
				patchBuilder.WriteString(l + "\n")
			}
		}

		res[i] = types.FileDiff{
			Path:      p.NewPath,
			OldPath:   p.OldPath,
			Status:    p.Status,
			Additions: p.Additions,
			Deletions: p.Deletions,
			Patch:     patchBuilder.String(),
			IsBinary:  p.IsBinary,
		}
	}
	return res
}

// FormatCompactSummary returns a 1-line diff summary (e.g. "+124 -35 in 5 files").
func (s DiffStats) FormatCompactSummary() string {
	return fmt.Sprintf("+%d -%d across %d files", s.Additions, s.Deletions, s.Files)
}
