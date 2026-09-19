// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package dedup

import (
	"os"
	"strconv"
	"strings"
	"unicode"
)

// CandidateFinding represents a code review suggestion to evaluate for deduplication.
type CandidateFinding struct {
	ID                 string
	FilePath           string
	LineStart          int
	SuggestionContent  string
	OneSentenceSummary string
}

// TokenizeWords splits a string into lowercase alphanumeric words for token similarity.
func TokenizeWords(s string) map[string]struct{} {
	words := make(map[string]struct{})
	f := func(c rune) bool {
		return !unicode.IsLetter(c) && !unicode.IsNumber(c)
	}
	parts := strings.FieldsFunc(strings.ToLower(s), f)
	for _, p := range parts {
		if len(p) > 2 {
			words[p] = struct{}{}
		}
	}
	return words
}

// JaccardSimilarity computes word-overlap coefficient between two text passages.
func JaccardSimilarity(a, b string) float64 {
	tokensA := TokenizeWords(a)
	tokensB := TokenizeWords(b)

	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	intersection := 0
	for k := range tokensA {
		if _, ok := tokensB[k]; ok {
			intersection++
		}
	}

	union := len(tokensA) + len(tokensB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// DefaultDedupThreshold is calibrated from benchmark golden dataset (0.3).
const DefaultDedupThreshold = 0.3

// DedupThreshold returns the runtime calibrated threshold.
func DedupThreshold() float64 {
	if v := os.Getenv("DEDUP_CONTENT_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f < 1 {
			return f
		}
	}
	return DefaultDedupThreshold
}

// CollapseNearDuplicates identifies and clusters overlapping findings on the same file/region.
// It ensures that duplicate phrasings of the same issue do not result in multi-comment spam.
func CollapseNearDuplicates(candidates []CandidateFinding, threshold ...float64) []CandidateFinding {
	thresh := DedupThreshold()
	if len(threshold) > 0 && threshold[0] > 0 {
		thresh = threshold[0]
	}

	if len(candidates) <= 1 {
		return candidates
	}

	var kept []CandidateFinding

	for _, cand := range candidates {
		isDuplicate := false
		for _, existing := range kept {
			// Compare if on the same file within proximity (e.g. within 10 lines)
			if cand.FilePath == existing.FilePath && abs(cand.LineStart-existing.LineStart) <= 10 {
				sim := JaccardSimilarity(cand.SuggestionContent, existing.SuggestionContent)
				if sim >= thresh {
					isDuplicate = true
					break
				}
			}
		}

		if !isDuplicate {
			kept = append(kept, cand)
		}
	}

	return kept
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
