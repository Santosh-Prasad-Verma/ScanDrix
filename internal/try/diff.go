package try

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	diffGitRegex   = regexp.MustCompile(`^diff --git (?:"a/|a/)(.+?)"? (?:"b/|b/)(.+?)"?$`)
	hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)
)

// ParseUnifiedDiff parses a unified diff text into a structured slice of DiffFile.
func ParseUnifiedDiff(diffText string) []DiffFile {
	if strings.TrimSpace(diffText) == "" {
		return nil
	}

	var files []DiffFile
	var current *DiffFile
	var currentHunk *DiffHunk
	var newLineCursor int
	var oldLineCursor int

	lines := strings.Split(diffText, "\n")

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")

		if strings.HasPrefix(line, "diff --git") {
			matches := diffGitRegex.FindStringSubmatch(line)
			var oldPath *string
			newPath := "(unknown)"

			if len(matches) >= 3 {
				old := matches[1]
				nw := matches[2]
				newPath = nw
				if old != nw {
					oldPath = &old
				}
			} else if len(matches) == 2 {
				newPath = matches[1]
			}

			files = append(files, DiffFile{
				Path:      newPath,
				OldPath:   oldPath,
				Status:    FileStatusModified,
				Additions: 0,
				Deletions: 0,
				Hunks:     nil,
			})
			current = &files[len(files)-1]
			currentHunk = nil
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "new file mode") {
			current.Status = FileStatusAdded
			continue
		}
		if strings.HasPrefix(line, "deleted file mode") {
			current.Status = FileStatusDeleted
			continue
		}
		if strings.HasPrefix(line, "rename from") || strings.HasPrefix(line, "rename to") {
			current.Status = FileStatusRenamed
			continue
		}

		if strings.HasPrefix(line, "@@") {
			matches := hunkHeaderRegex.FindStringSubmatch(line)
			oldLineCursor = 0
			newLineCursor = 0
			if len(matches) >= 3 {
				if o, err := strconv.Atoi(matches[1]); err == nil {
					oldLineCursor = o
				}
				if n, err := strconv.Atoi(matches[2]); err == nil {
					newLineCursor = n
				}
			}
			current.Hunks = append(current.Hunks, DiffHunk{
				Header: line,
				Lines:  nil,
			})
			currentHunk = &current.Hunks[len(current.Hunks)-1]
			continue
		}

		if currentHunk == nil {
			continue
		}

		// Skip header lines preceding first hunk
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		if strings.HasPrefix(line, "+") {
			nl := newLineCursor
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineAdd,
				Text:    line[1:],
				NewLine: &nl,
				OldLine: nil,
			})
			current.Additions++
			newLineCursor++
		} else if strings.HasPrefix(line, "-") {
			ol := oldLineCursor
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineDel,
				Text:    line[1:],
				NewLine: nil,
				OldLine: &ol,
			})
			current.Deletions++
			oldLineCursor++
		} else if strings.HasPrefix(line, " ") {
			nl := newLineCursor
			ol := oldLineCursor
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineContext,
				Text:    line[1:],
				NewLine: &nl,
				OldLine: &ol,
			})
			newLineCursor++
			oldLineCursor++
		} else if strings.HasPrefix(line, "\\") {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    DiffLineContext,
				Text:    line,
				NewLine: nil,
				OldLine: nil,
			})
		}
	}

	return files
}
