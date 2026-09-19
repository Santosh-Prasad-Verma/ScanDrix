package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
)

// SafeguardTriageService implements domain.ISafeguardService.
type SafeguardTriageService struct{}

// NewSafeguardTriageService constructs a safeguard triage service.
func NewSafeguardTriageService() *SafeguardTriageService {
	return &SafeguardTriageService{}
}

// FilterSuggestions inspects candidate suggestions against file patches to eliminate hallucinations and out-of-bounds diffs.
func (s *SafeguardTriageService) FilterSuggestions(ctx context.Context, suggestions []domain.CodeSuggestion, patches []*diff.FilePatch) ([]domain.CodeSuggestion, []domain.CodeSuggestion, error) {
	patchMap := make(map[string]*diff.FilePatch)
	for _, p := range patches {
		patchMap[p.NewPath] = p
		if p.OldPath != "" && p.OldPath != p.NewPath {
			patchMap[p.OldPath] = p
		}
	}

	var valid []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion
	seenSpans := make(map[string]bool)

	for _, sug := range suggestions {
		// 1. Must have valid file and description
		if strings.TrimSpace(sug.RelevantFile) == "" || strings.TrimSpace(sug.SuggestionContent) == "" {
			sug.PriorityStatus = domain.PriorityStatusDiscardedBySafeguard
			discarded = append(discarded, sug)
			continue
		}

		// 2. Start line must be positive and <= end line
		if sug.RelevantLinesStart <= 0 || (sug.RelevantLinesEnd > 0 && sug.RelevantLinesStart > sug.RelevantLinesEnd) {
			sug.PriorityStatus = domain.PriorityStatusDiscardedBySafeguard
			discarded = append(discarded, sug)
			continue
		}

		// 3. Patch existence check
		patch, exists := patchMap[sug.RelevantFile]
		if !exists {
			sug.PriorityStatus = domain.PriorityStatusDiscardedByCodeDiff
			discarded = append(discarded, sug)
			continue
		}

		// 4. Validate lines overlap modified lines in patch
		overlaps := false
		for _, h := range patch.Hunks {
			hunkStart := h.NewStart
			hunkEnd := h.NewStart + h.NewLines - 1
			if sug.RelevantLinesStart <= hunkEnd && (sug.RelevantLinesEnd == 0 || sug.RelevantLinesEnd >= hunkStart) {
				overlaps = true
				break
			}
		}

		if !overlaps {
			sug.PriorityStatus = domain.PriorityStatusDiscardedByCodeDiff
			discarded = append(discarded, sug)
			continue
		}

		// 5. Deduplication check on identical line span + category
		dedupKey := fmt.Sprintf("%s:%d-%d:%s", sug.RelevantFile, sug.RelevantLinesStart, sug.RelevantLinesEnd, sug.Category)
		if seenSpans[dedupKey] {
			sug.PriorityStatus = domain.PriorityStatusDiscardedBySafeguard
			discarded = append(discarded, sug)
			continue
		}
		seenSpans[dedupKey] = true

		sug.PriorityStatus = domain.PriorityStatusPrioritized
		valid = append(valid, sug)
	}

	return valid, discarded, nil
}
