// Package chaos provides SCM chaos fault injection, resiliency primitives, and benchmark matrices.
package chaos

import (
	"fmt"
	"strings"
)

const (
	// MaxGitHubCommentLength is GitHub's hard payload boundary (65,536 characters).
	MaxGitHubCommentLength = 65536

	// SafeCommentChunkSize reserves buffer space for part headers, footers, and code fences.
	SafeCommentChunkSize = 60000

	// MaxGitLabNoteLength is GitLab's markdown note limit.
	MaxGitLabNoteLength = 1000000
)

// CommentPayloadChunker ensures long review summaries and large diff suggestions never exceed SCM platform boundaries.
type CommentPayloadChunker struct{}

// NewCommentPayloadChunker creates a comment chunker.
func NewCommentPayloadChunker() *CommentPayloadChunker {
	return &CommentPayloadChunker{}
}

// SplitComment chunks a long markdown comment into valid multi-part segments respecting code fence integrity.
func (c *CommentPayloadChunker) SplitComment(rawContent string, platform SCMPlatform) []string {
	limit := SafeCommentChunkSize
	if platform == PlatformGitLab {
		limit = MaxGitLabNoteLength - 5000
	}

	if len(rawContent) <= limit {
		return []string{rawContent}
	}

	lines := strings.Split(rawContent, "\n")
	var chunks []string
	var currentChunk strings.Builder
	inCodeBlock := false
	codeFenceLang := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				inCodeBlock = false
				codeFenceLang = ""
			} else {
				inCodeBlock = true
				codeFenceLang = strings.TrimPrefix(trimmed, "```")
			}
		}

		// Check if adding this line exceeds the safe chunk size
		if currentChunk.Len()+len(line)+1 > limit {
			if inCodeBlock {
				// Close the code block in current chunk
				currentChunk.WriteString("\n```\n")
			}

			chunks = append(chunks, currentChunk.String())
			currentChunk.Reset()

			if inCodeBlock {
				// Re-open the code block in next chunk
				currentChunk.WriteString("```" + codeFenceLang + "\n")
			}
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n")
		}
		currentChunk.WriteString(line)
	}

	if currentChunk.Len() > 0 {
		chunks = append(chunks, currentChunk.String())
	}

	// Add part pagination header/footer if multi-part
	if len(chunks) > 1 {
		for i := range chunks {
			header := fmt.Sprintf("*(Part %d of %d)*\n\n", i+1, len(chunks))
			chunks[i] = header + chunks[i]
		}
	}

	return chunks
}

// ValidateDiffHunkPosition checks if a comment's target line is within the diff hunk boundaries.
// GitHub returns 422 Unprocessable Entity if comments target lines outside the modified diff hunk.
func ValidateDiffHunkPosition(startLine, endLine int, modifiedLineRanges [][2]int) bool {
	if startLine <= 0 || endLine <= 0 || startLine > endLine {
		return false
	}

	// The finding must touch at least one modified line
	for _, r := range modifiedLineRanges {
		rStart, rEnd := r[0], r[1]
		if !(endLine < rStart || startLine > rEnd) {
			return true
		}
	}

	return false
}
