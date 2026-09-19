package operations

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FileChangeType denotes the nature of file modification in a diff.
type FileChangeType string

const (
	ChangeModified FileChangeType = "MODIFIED"
	ChangeAdded    FileChangeType = "ADDED"
	ChangeDeleted  FileChangeType = "DELETED"
	ChangeRenamed  FileChangeType = "RENAMED"
	ChangeCopied   FileChangeType = "COPIED"
)

// DiffHunkLine represents a single line inside a unified diff hunk.
type DiffHunkLine struct {
	Type    string `json:"type"` // "+", "-", " "
	Content string `json:"content"`
	OldLine int    `json:"oldLine,omitempty"`
	NewLine int    `json:"newLine,omitempty"`
}

// DiffHunk captures a single contiguous block of code changes.
type DiffHunk struct {
	Header   string         `json:"header"` // @@ -1,5 +1,6 @@
	OldStart int            `json:"oldStart"`
	OldLines int            `json:"oldLines"`
	NewStart int            `json:"newStart"`
	NewLines int            `json:"newLines"`
	Lines    []DiffHunkLine `json:"lines"`
}

// FileDiffSummary summarizes changes to an individual file within a diff stream.
type FileDiffSummary struct {
	OldPath     string         `json:"oldPath"`
	NewPath     string         `json:"newPath"`
	ChangeType  FileChangeType `json:"changeType"`
	IsBinary    bool           `json:"isBinary"`
	IsGenerated bool           `json:"isGenerated"`
	Language    string         `json:"language"`
	Additions   int            `json:"additions"`
	Deletions   int            `json:"deletions"`
	Hunks       []DiffHunk     `json:"hunks"`
}

// DiffStreamSummary aggregates total diff metrics across all parsed files.
type DiffStreamSummary struct {
	TotalFiles       int               `json:"totalFiles"`
	TotalAdditions   int               `json:"totalAdditions"`
	TotalDeletions   int               `json:"totalDeletions"`
	Files            []FileDiffSummary `json:"files"`
	Languages        map[string]int    `json:"languages"`
	HasBinaryFiles   bool              `json:"hasBinaryFiles"`
	GeneratedFiles   int               `json:"generatedFiles"`
}

var hunkHeaderRegex = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// DetectLanguage infers programming language by file extension.
func DetectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".cs":
		return "C#"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	case ".sql":
		return "SQL"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".md", ".markdown":
		return "Markdown"
	case ".sh", ".bash":
		return "Shell"
	case ".dockerfile", "dockerfile":
		return "Dockerfile"
	default:
		return "Text"
	}
}

// IsGeneratedFile checks whether a file is an automatically generated asset or lockfile.
func IsGeneratedFile(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".pb.go") ||
		strings.HasSuffix(lower, ".generated.go") ||
		strings.HasSuffix(lower, ".min.js") ||
		strings.HasSuffix(lower, ".min.css") ||
		strings.HasSuffix(lower, "package-lock.json") ||
		strings.HasSuffix(lower, "yarn.lock") ||
		strings.HasSuffix(lower, "pnpm-lock.yaml") ||
		strings.HasSuffix(lower, "cargo.lock") ||
		strings.HasSuffix(lower, "go.sum") ||
		strings.Contains(lower, "/generated/") ||
		strings.Contains(lower, "/gen/") {
		return true
	}
	return false
}

// ParseDiffStream reads and tokenizes a unified diff or patch stream in a memory-efficient manner.
func ParseDiffStream(r io.Reader) (*DiffStreamSummary, error) {
	scanner := bufio.NewScanner(r)
	// Support long lines (up to 1MB)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	summary := &DiffStreamSummary{
		Files:     make([]FileDiffSummary, 0),
		Languages: make(map[string]int),
	}

	var currentFile *FileDiffSummary
	var currentHunk *DiffHunk
	oldLineNum := 0
	newLineNum := 0

	flushHunk := func() {
		if currentHunk != nil && currentFile != nil {
			currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			currentHunk = nil
		}
	}

	flushFile := func() {
		flushHunk()
		if currentFile != nil {
			summary.Files = append(summary.Files, *currentFile)
			summary.TotalFiles++
			summary.TotalAdditions += currentFile.Additions
			summary.TotalDeletions += currentFile.Deletions
			if currentFile.IsBinary {
				summary.HasBinaryFiles = true
			}
			if currentFile.IsGenerated {
				summary.GeneratedFiles++
			}
			if currentFile.Language != "" {
				summary.Languages[currentFile.Language]++
			}
			currentFile = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// New file diff header: diff --git a/foo b/bar
		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			parts := strings.Split(line, " ")
			oldPath := ""
			newPath := ""
			if len(parts) >= 4 {
				oldPath = strings.TrimPrefix(parts[2], "a/")
				newPath = strings.TrimPrefix(parts[3], "b/")
			}
			currentFile = &FileDiffSummary{
				OldPath:     oldPath,
				NewPath:     newPath,
				ChangeType:  ChangeModified,
				Language:    DetectLanguage(newPath),
				IsGenerated: IsGeneratedFile(newPath),
				Hunks:       make([]DiffHunk, 0),
			}
			continue
		}

		if currentFile == nil {
			continue
		}

		if strings.HasPrefix(line, "new file mode ") {
			currentFile.ChangeType = ChangeAdded
			continue
		}
		if strings.HasPrefix(line, "deleted file mode ") {
			currentFile.ChangeType = ChangeDeleted
			continue
		}
		if strings.HasPrefix(line, "similarity index ") {
			currentFile.ChangeType = ChangeRenamed
			continue
		}
		if strings.HasPrefix(line, "Binary files ") {
			currentFile.IsBinary = true
			continue
		}

		// Hunk header
		if strings.HasPrefix(line, "@@ ") {
			flushHunk()
			matches := hunkHeaderRegex.FindStringSubmatch(line)
			if len(matches) >= 4 {
				oldStart, _ := strconv.Atoi(matches[1])
				oldLines := 1
				if matches[2] != "" {
					oldLines, _ = strconv.Atoi(matches[2])
				}
				newStart, _ := strconv.Atoi(matches[3])
				newLines := 1
				if len(matches) >= 5 && matches[4] != "" {
					newLines, _ = strconv.Atoi(matches[4])
				}

				currentHunk = &DiffHunk{
					Header:   line,
					OldStart: oldStart,
					OldLines: oldLines,
					NewStart: newStart,
					NewLines: newLines,
					Lines:    make([]DiffHunkLine, 0),
				}
				oldLineNum = oldStart
				newLineNum = newStart
			}
			continue
		}

		// Inside hunk
		if currentHunk != nil {
			if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
				currentFile.Additions++
				currentHunk.Lines = append(currentHunk.Lines, DiffHunkLine{
					Type:    "+",
					Content: line[1:],
					NewLine: newLineNum,
				})
				newLineNum++
			} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
				currentFile.Deletions++
				currentHunk.Lines = append(currentHunk.Lines, DiffHunkLine{
					Type:    "-",
					Content: line[1:],
					OldLine: oldLineNum,
				})
				oldLineNum++
			} else if strings.HasPrefix(line, " ") {
				currentHunk.Lines = append(currentHunk.Lines, DiffHunkLine{
					Type:    " ",
					Content: line[1:],
					OldLine: oldLineNum,
					NewLine: newLineNum,
				})
				oldLineNum++
				newLineNum++
			}
		}
	}

	flushFile()

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("diff stream parse error: %w", err)
	}

	return summary, nil
}
