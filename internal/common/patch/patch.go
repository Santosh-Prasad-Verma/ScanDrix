package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const maxPatchLines = 50000

var reHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@[ ]?(.*)`)
var reDiffHunkHeader = regexp.MustCompile(`@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
var reLineMatch = regexp.MustCompile(`^(\d+) ([+-])`)
var reAddedLine = regexp.MustCompile(`^\s*(\d+)\s\+`)

type ModifiedRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type FileInput struct {
	Filename string `json:"filename"`
}

func HandlePatchDeletions(patchText, fileName, editType string) *string {
	if patchText == "" && editType != "modified" && editType != "added" {
		return nil
	}
	patchLines := strings.Split(patchText, "\n")
	patchNew := omitDeletionHunks(patchLines)
	if patchText != patchNew {
		return &patchNew
	}
	return &patchText
}

func omitDeletionHunks(patchLines []string) string {
	var tempHunk []string
	var addedPatched []string
	addHunk := false
	insideHunk := false

	for _, line := range patchLines {
		if strings.HasPrefix(line, "@@") {
			if reHunkHeader.MatchString(line) {
				if insideHunk && addHunk {
					addedPatched = append(addedPatched, tempHunk...)
					tempHunk = nil
					addHunk = false
				}
				tempHunk = append(tempHunk, line)
				insideHunk = true
			}
		} else {
			tempHunk = append(tempHunk, line)
			if len(line) > 0 && line[0] == '+' {
				addHunk = true
			}
		}
	}

	if insideHunk && addHunk {
		addedPatched = append(addedPatched, tempHunk...)
	}

	return strings.Join(addedPatched, "\n")
}

func ConvertToHunksWithLinesNumbers(patchText string, file FileInput) string {
	patchWithLinesStr := fmt.Sprintf("\n\n## file: '%s'\n", strings.TrimSpace(file.Filename))
	patchLines := strings.Split(patchText, "\n")
	if len(patchLines) > maxPatchLines {
		patchLines = patchLines[:maxPatchLines]
	}

	var newContentLines []string
	var oldContentLines []string
	var match []string
	start2 := -1
	prevHeaderLine := ""
	headerLine := ""

	for _, line := range patchLines {
		if strings.Contains(strings.ToLower(line), "no newline at end of file") {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			headerLine = line
			match = reHunkHeader.FindStringSubmatch(line)

			if match != nil && (len(newContentLines) > 0 || len(oldContentLines) > 0) {
				if prevHeaderLine != "" {
					patchWithLinesStr += fmt.Sprintf("\n%s\n", prevHeaderLine)
				}
				if len(newContentLines) > 0 {
					isPlusLines := false
					for _, l := range newContentLines {
						if strings.HasPrefix(l, "+") {
							isPlusLines = true
							break
						}
					}
					if isPlusLines {
						patchWithLinesStr = strings.TrimRight(patchWithLinesStr, " \t\r\n") + "\n__new hunk__\n"
						for i, l := range newContentLines {
							patchWithLinesStr += fmt.Sprintf("%d %s\n", start2+i, l)
						}
					}
				}
				if len(oldContentLines) > 0 {
					isMinusLines := false
					for _, l := range oldContentLines {
						if strings.HasPrefix(l, "-") {
							isMinusLines = true
							break
						}
					}
					if isMinusLines {
						patchWithLinesStr = strings.TrimRight(patchWithLinesStr, " \t\r\n") + "\n__old hunk__\n"
						for _, lineOld := range oldContentLines {
							patchWithLinesStr += fmt.Sprintf("%s\n", lineOld)
						}
					}
				}
				newContentLines = nil
				oldContentLines = nil
			}

			if match != nil {
				prevHeaderLine = headerLine
				if val, err := strconv.Atoi(match[3]); err == nil {
					start2 = val
				}
			}
		} else if strings.HasPrefix(line, "+") {
			newContentLines = append(newContentLines, line)
		} else if strings.HasPrefix(line, "-") {
			oldContentLines = append(oldContentLines, line)
		} else {
			newContentLines = append(newContentLines, line)
			oldContentLines = append(oldContentLines, line)
		}
	}

	if match != nil && len(newContentLines) > 0 {
		patchWithLinesStr += fmt.Sprintf("\n%s\n", headerLine)
		isPlusLines := false
		for _, l := range newContentLines {
			if strings.HasPrefix(l, "+") {
				isPlusLines = true
				break
			}
		}
		if isPlusLines {
			patchWithLinesStr = strings.TrimRight(patchWithLinesStr, " \t\r\n") + "\n__new hunk__\n"
			for i, l := range newContentLines {
				patchWithLinesStr += fmt.Sprintf("%d %s\n", start2+i, l)
			}
		}
		if len(oldContentLines) > 0 {
			isMinusLines := false
			for _, l := range oldContentLines {
				if strings.HasPrefix(l, "-") {
					isMinusLines = true
					break
				}
			}
			if isMinusLines {
				patchWithLinesStr = strings.TrimRight(patchWithLinesStr, " \t\r\n") + "\n__old hunk__\n"
				for _, lineOld := range oldContentLines {
					patchWithLinesStr += fmt.Sprintf("%s\n", lineOld)
				}
			}
		}
	}

	return strings.TrimSpace(patchWithLinesStr)
}

func ConvertToUnifiedDiffWithLineNumbers(patchText string, file FileInput) string {
	if patchText == "" {
		return ""
	}

	lines := strings.Split(patchText, "\n")
	if len(lines) > maxPatchLines {
		lines = lines[:maxPatchLines]
	}

	result := []string{fmt.Sprintf("## file: '%s'", strings.TrimSpace(file.Filename))}
	newLine := 0

	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "no newline at end of file") {
			continue
		}

		match := reHunkHeader.FindStringSubmatch(line)
		if match != nil {
			if val, err := strconv.Atoi(match[3]); err == nil {
				newLine = val
			}
			result = append(result, line)
			continue
		}

		if strings.HasPrefix(line, "+") {
			result = append(result, fmt.Sprintf("%6d %s", newLine, line))
			newLine++
		} else if strings.HasPrefix(line, "-") {
			result = append(result, fmt.Sprintf("%6s %s", "", line))
		} else {
			result = append(result, fmt.Sprintf("%6d %s", newLine, line))
			newLine++
		}
	}

	return strings.Join(result, "\n")
}

func ExtractLinesFromDiffHunk(diffHunk string) []ModifiedRange {
	if diffHunk == "" {
		return nil
	}

	lines := strings.Split(diffHunk, "\n")
	var modifiedRanges []ModifiedRange
	var currentRange *ModifiedRange

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			match := reDiffHunkHeader.FindStringSubmatch(line)
			if match != nil {
				if currentRange != nil {
					modifiedRanges = append(modifiedRanges, *currentRange)
					currentRange = nil
				}
			}
			continue
		}

		if strings.Contains(line, "__new hunk__") || strings.Contains(line, "__old hunk__") {
			continue
		}

		lineMatch := reLineMatch.FindStringSubmatch(line)
		if lineMatch != nil {
			lineNumber, _ := strconv.Atoi(lineMatch[1])
			changeType := lineMatch[2]

			if changeType == "+" {
				if currentRange == nil {
					currentRange = &ModifiedRange{Start: lineNumber, End: lineNumber}
				} else if lineNumber == currentRange.End+1 {
					currentRange.End = lineNumber
				} else {
					modifiedRanges = append(modifiedRanges, *currentRange)
					currentRange = &ModifiedRange{Start: lineNumber, End: lineNumber}
				}
			}
		} else {
			if currentRange != nil {
				modifiedRanges = append(modifiedRanges, *currentRange)
				currentRange = nil
			}
		}
	}

	if currentRange != nil {
		modifiedRanges = append(modifiedRanges, *currentRange)
	}

	return modifiedRanges
}

func ExtractLinesFromUnifiedDiff(diffHunk string) []ModifiedRange {
	if diffHunk == "" {
		return nil
	}

	lines := strings.Split(diffHunk, "\n")
	var modifiedRanges []ModifiedRange
	var currentRange *ModifiedRange

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "## file:") {
			if currentRange != nil {
				modifiedRanges = append(modifiedRanges, *currentRange)
				currentRange = nil
			}
			continue
		}

		addedMatch := reAddedLine.FindStringSubmatch(line)
		if addedMatch != nil {
			lineNumber, _ := strconv.Atoi(addedMatch[1])

			if currentRange == nil {
				currentRange = &ModifiedRange{Start: lineNumber, End: lineNumber}
			} else if lineNumber == currentRange.End+1 {
				currentRange.End = lineNumber
			} else {
				modifiedRanges = append(modifiedRanges, *currentRange)
				currentRange = &ModifiedRange{Start: lineNumber, End: lineNumber}
			}
		} else {
			if currentRange != nil {
				modifiedRanges = append(modifiedRanges, *currentRange)
				currentRange = nil
			}
		}
	}

	if currentRange != nil {
		modifiedRanges = append(modifiedRanges, *currentRange)
	}

	return modifiedRanges
}
