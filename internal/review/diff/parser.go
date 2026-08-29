package diff

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// LineType classifies a diff line.
type LineType int

const (
	LineContext LineType = iota
	LineAddition
	LineDeletion
)

// DiffLine represents a single line inside a unified diff hunk.
type DiffLine struct {
	Type      LineType
	OldLineNo int
	NewLineNo int
	Content   string
}

// Hunk represents a cohesive block of line changes.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Header   string
	Lines    []DiffLine
}

// FilePatch represents the complete diff for a single file.
type FilePatch struct {
	OldPath   string
	NewPath   string
	IsBinary  bool
	IsNew     bool
	IsDeleted bool
	Additions int
	Deletions int
	Hunks     []Hunk
}

var (
	diffHeaderRegex = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	hunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)
)

// ParseUnifiedDiff parses a multi-file unified git diff stream into structured FilePatch slices.
func ParseUnifiedDiff(r io.Reader) ([]*FilePatch, error) {
	scanner := bufio.NewScanner(r)
	// Support long lines in generated files or minified bundles up to 1MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var patches []*FilePatch
	var currentPatch *FilePatch
	var currentHunk *Hunk
	var oldLineTracker, newLineTracker int

	for scanner.Scan() {
		line := scanner.Text()

		// Check for diff header
		if matches := diffHeaderRegex.FindStringSubmatch(line); len(matches) == 3 {
			if currentHunk != nil && currentPatch != nil {
				currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
				currentHunk = nil
			}
			if currentPatch != nil {
				patches = append(patches, currentPatch)
			}
			currentPatch = &FilePatch{
				OldPath: matches[1],
				NewPath: matches[2],
			}
			continue
		}

		if currentPatch == nil {
			continue
		}

		// Detect file lifecycle attributes
		if strings.HasPrefix(line, "new file mode") {
			currentPatch.IsNew = true
			continue
		}
		if strings.HasPrefix(line, "deleted file mode") {
			currentPatch.IsDeleted = true
			continue
		}
		if strings.HasPrefix(line, "Binary files") {
			currentPatch.IsBinary = true
			continue
		}

		// Check for hunk header: @@ -1,5 +1,6 @@
		if matches := hunkHeaderRegex.FindStringSubmatch(line); len(matches) >= 5 {
			if currentHunk != nil {
				currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
			}

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
			// Empty context line
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
			currentPatch.Additions++
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineAddition,
				NewLineNo: newLineTracker,
				Content:   content,
			})
			newLineTracker++
		case '-':
			currentPatch.Deletions++
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineDeletion,
				OldLineNo: oldLineTracker,
				Content:   content,
			})
			oldLineTracker++
		case ' ':
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:      LineContext,
				OldLineNo: oldLineTracker,
				NewLineNo: newLineTracker,
				Content:   content,
			})
			oldLineTracker++
			newLineTracker++
		case '\\':
			// "\ No newline at end of file" - ignore
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading diff stream: %w", err)
	}

	if currentHunk != nil && currentPatch != nil {
		currentPatch.Hunks = append(currentPatch.Hunks, *currentHunk)
	}
	if currentPatch != nil {
		patches = append(patches, currentPatch)
	}

	return patches, nil
}
