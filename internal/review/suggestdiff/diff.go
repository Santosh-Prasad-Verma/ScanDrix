package suggestdiff

import (
	"fmt"
	"strings"
)

// DiffLineType specifies whether a diff line is context, added, or deleted.
type DiffLineType int

const (
	DiffContext DiffLineType = iota
	DiffAdded
	DiffDeleted
)

// DiffLine represents a single line in a computed line-level diff.
type DiffLine struct {
	Type    DiffLineType `json:"type"`
	Content string       `json:"content"`
}

// ComputeLineDiff computes a line-level diff using Myers LCS (Longest Common Subsequence).
func ComputeLineDiff(oldLines, newLines []string) []DiffLine {
	m, n := len(oldLines), len(newLines)
	if m == 0 && n == 0 {
		return nil
	}

	// 1. Build LCS Dynamic Programming Table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if strings.TrimSpace(oldLines[i-1]) == strings.TrimSpace(newLines[j-1]) {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else if lcs[i-1][j] >= lcs[i][j-1] {
				lcs[i][j] = lcs[i-1][j]
			} else {
				lcs[i][j] = lcs[i][j-1]
			}
		}
	}

	// 2. Backtrack to produce sequence
	var reversed []DiffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && strings.TrimSpace(oldLines[i-1]) == strings.TrimSpace(newLines[j-1]) {
			if oldLines[i-1] == newLines[j-1] {
				reversed = append(reversed, DiffLine{Type: DiffContext, Content: oldLines[i-1]})
			} else {
				// Preserve exact indentation/whitespace changes
				reversed = append(reversed, DiffLine{Type: DiffAdded, Content: newLines[j-1]})
				reversed = append(reversed, DiffLine{Type: DiffDeleted, Content: oldLines[i-1]})
			}
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			reversed = append(reversed, DiffLine{Type: DiffAdded, Content: newLines[j-1]})
			j--
		} else if i > 0 {
			reversed = append(reversed, DiffLine{Type: DiffDeleted, Content: oldLines[i-1]})
			i--
		}
	}

	// 3. Reverse to restore original top-to-bottom line order
	result := make([]DiffLine, len(reversed))
	for idx, line := range reversed {
		result[len(reversed)-1-idx] = line
	}

	return result
}

// RenderUnifiedDiff renders computed diff lines into standard git patch format.
func RenderUnifiedDiff(oldCode, newCode, filePath string) string {
	oldLines := strings.Split(oldCode, "\n")
	newLines := strings.Split(newCode, "\n")
	diffLines := ComputeLineDiff(oldLines, newLines)

	if len(diffLines) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", filePath, filePath))
	sb.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines)))

	for _, dl := range diffLines {
		switch dl.Type {
		case DiffAdded:
			sb.WriteString("+" + dl.Content + "\n")
		case DiffDeleted:
			sb.WriteString("-" + dl.Content + "\n")
		case DiffContext:
			sb.WriteString(" " + dl.Content + "\n")
		}
	}

	return sb.String()
}

// RenderANSI formats diff lines with standard terminal ANSI colors for CLI and TUI triage.
func RenderANSI(lines []DiffLine) string {
	const (
		colorReset = "\033[0m"
		colorRed   = "\033[31m"
		colorGreen = "\033[32m"
		colorDim   = "\033[2m"
	)

	var sb strings.Builder
	for _, l := range lines {
		switch l.Type {
		case DiffAdded:
			sb.WriteString(colorGreen + "+" + l.Content + colorReset + "\n")
		case DiffDeleted:
			sb.WriteString(colorRed + "-" + l.Content + colorReset + "\n")
		case DiffContext:
			sb.WriteString(colorDim + " " + l.Content + colorReset + "\n")
		}
	}
	return sb.String()
}
