// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package anchoring

import (
	"fmt"
	"strings"
)

// DiffHunk represents an active diff change range.
type DiffHunk struct {
	FilePath  string
	OldStart  int
	OldLength int
	NewStart  int
	NewLength int
	Lines     []string
}

// CommentAnchor represents a code review comment's line attachment.
type CommentAnchor struct {
	FilePath  string
	LineStart int
	LineEnd   int
}

// AnchorEvaluationResult captures the evaluation metric for a comment.
type AnchorEvaluationResult struct {
	Valid          bool
	IsInsideHunk   bool
	Confidence     float64
	FailureReason  string
	NormalizedFile string
}

// NormalizePath canonicalizes file paths across OS boundaries.
func NormalizePath(p string) string {
	if p == "" {
		return ""
	}
	clean := strings.ReplaceAll(p, "\\", "/")
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "/")
	return strings.ToLower(clean)
}

// EvaluateAnchor verifies that a suggested comment anchors precisely to an actual changed hunk.
func EvaluateAnchor(anchor CommentAnchor, hunks []DiffHunk) AnchorEvaluationResult {
	normTarget := NormalizePath(anchor.FilePath)
	if normTarget == "" {
		return AnchorEvaluationResult{
			Valid:         false,
			FailureReason: "missing or empty file path in comment anchor",
		}
	}

	var matchingHunks []DiffHunk
	for _, h := range hunks {
		if NormalizePath(h.FilePath) == normTarget {
			matchingHunks = append(matchingHunks, h)
		}
	}

	if len(matchingHunks) == 0 {
		return AnchorEvaluationResult{
			Valid:          false,
			NormalizedFile: normTarget,
			FailureReason:  fmt.Sprintf("file %s is not in changed diff hunks", anchor.FilePath),
		}
	}

	// Verify line overlap
	for _, h := range matchingHunks {
		hunkEnd := h.NewStart + h.NewLength
		if anchor.LineStart >= h.NewStart && anchor.LineStart <= hunkEnd {
			return AnchorEvaluationResult{
				Valid:          true,
				IsInsideHunk:   true,
				Confidence:     1.0,
				NormalizedFile: normTarget,
			}
		}
	}

	// Finding is on a changed file, but on an unchanged line outside the hunk
	return AnchorEvaluationResult{
		Valid:          false,
		IsInsideHunk:   false,
		Confidence:     0.4,
		NormalizedFile: normTarget,
		FailureReason:  fmt.Sprintf("line %d is outside modified hunk ranges for %s", anchor.LineStart, anchor.FilePath),
	}
}
