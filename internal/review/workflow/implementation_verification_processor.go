// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
)

// VerificationOutcome summarizes the result of verifying one code suggestion.
type VerificationOutcome struct {
	SuggestionID         uuid.UUID                   `json:"suggestion_id"`
	FilePath             string                      `json:"file_path"`
	OldStatus            domain.ImplementationStatus `json:"old_status"`
	NewStatus            domain.ImplementationStatus `json:"new_status"`
	MatchedRatio         float64                     `json:"matched_ratio"` // 0.0 to 1.0
	AutoResolvedThread   bool                        `json:"auto_resolved_thread"`
	Reason               string                      `json:"reason"`
}

// VerificationBatchReport provides aggregate statistics for a commit verification run.
type VerificationBatchReport struct {
	PullNumber           int                   `json:"pull_number"`
	CommitSHA            string                `json:"commit_sha"`
	TotalEvaluated       int                   `json:"total_evaluated"`
	ImplementedCount     int                   `json:"implemented_count"`
	PartialCount         int                   `json:"partial_count"`
	IgnoredCount         int                   `json:"ignored_count"`
	AutoResolvedCount    int                   `json:"auto_resolved_count"`
	ImplementationRatio  float64               `json:"implementation_ratio"`
	Outcomes             []VerificationOutcome `json:"outcomes"`
	VerifiedAt           time.Time             `json:"verified_at"`
}

// SuggestionStore defines the persistence interface required for verification.
type SuggestionStore interface {
	GetByPR(ctx context.Context, pullNumber int) ([]domain.CodeSuggestion, error)
	UpdateImplementationStatus(ctx context.Context, id uuid.UUID, status domain.ImplementationStatus) error
}

// SCMThreadResolver defines the remote inline comment thread resolution contract.
type SCMThreadResolver interface {
	ResolveThread(ctx context.Context, repoID string, pullNumber int, threadID string) error
}

// ImplementationVerificationProcessor inspects newly pushed commits to verify whether
// developers accepted and applied previous AI code suggestions.
type ImplementationVerificationProcessor struct {
	mu             sync.RWMutex
	store          SuggestionStore
	threadResolver SCMThreadResolver
}

// NewImplementationVerificationProcessor constructs the processor.
func NewImplementationVerificationProcessor(
	store SuggestionStore,
	resolver ...SCMThreadResolver,
) *ImplementationVerificationProcessor {
	var res SCMThreadResolver
	if len(resolver) > 0 {
		res = resolver[0]
	}
	return &ImplementationVerificationProcessor{
		store:          store,
		threadResolver: res,
	}
}

// VerifyCommitSuggestions checks all active suggestions against newly pushed file patches.
func (p *ImplementationVerificationProcessor) VerifyCommitSuggestions(
	ctx context.Context,
	repoID string,
	pullNumber int,
	commitSHA string,
	newPatches []pipeline.FileChangeInfo,
) (VerificationBatchReport, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	report := VerificationBatchReport{
		PullNumber: pullNumber,
		CommitSHA:  commitSHA,
		VerifiedAt: time.Now().UTC(),
		Outcomes:   make([]VerificationOutcome, 0),
	}

	if p.store == nil {
		return report, nil
	}

	// 1. Fetch historical suggestions previously posted on the PR
	suggestions, err := p.store.GetByPR(ctx, pullNumber)
	if err != nil {
		return report, fmt.Errorf("failed to fetch PR suggestions: %w", err)
	}

	if len(suggestions) == 0 {
		return report, nil
	}

	// 2. Index new patches by file path
	patchMap := make(map[string]pipeline.FileChangeInfo, len(newPatches))
	for _, np := range newPatches {
		norm := strings.TrimPrefix(np.Filename, "/")
		patchMap[norm] = np
	}

	// 3. Evaluate each suggestion
	for _, sug := range suggestions {
		// Only verify suggestions that were previously delivered to the SCM
		if sug.DeliveryStatus != domain.DeliveryStatusSent && sug.DeliveryStatus != domain.DeliveryStatusQueued {
			continue
		}

		report.TotalEvaluated++
		filePath := strings.TrimPrefix(sug.RelevantFile, "/")
		if filePath == "" {
			filePath = strings.TrimPrefix(sug.FilePath, "/")
		}

		patchInfo, fileModified := patchMap[filePath]
		if !fileModified {
			// File was not touched in this commit, status remains unchanged
			continue
		}

		outcome := p.evaluateSingleSuggestion(ctx, repoID, pullNumber, sug, patchInfo)
		report.Outcomes = append(report.Outcomes, outcome)

		switch outcome.NewStatus {
		case domain.ImplStatusImplemented:
			report.ImplementedCount++
			if outcome.AutoResolvedThread {
				report.AutoResolvedCount++
			}
		case domain.ImplStatusPartiallyImplemented:
			report.PartialCount++
		case domain.ImplStatusIgnored:
			report.IgnoredCount++
		}

		// Update database status
		_ = p.store.UpdateImplementationStatus(ctx, sug.ID, outcome.NewStatus)
	}

	if report.TotalEvaluated > 0 {
		report.ImplementationRatio = float64(report.ImplementedCount) / float64(report.TotalEvaluated)
	}

	return report, nil
}

func (p *ImplementationVerificationProcessor) evaluateSingleSuggestion(
	ctx context.Context,
	repoID string,
	pullNumber int,
	sug domain.CodeSuggestion,
	patchInfo pipeline.FileChangeInfo,
) VerificationOutcome {
	targetSnippet := strings.TrimSpace(sug.ImprovedCode)
	if targetSnippet == "" {
		targetSnippet = strings.TrimSpace(sug.SuggestedReplacement)
	}

	addedLines := extractAddedLines(patchInfo.Patch)
	deletedLines := extractDeletedLines(patchInfo.Patch)

	outcome := VerificationOutcome{
		SuggestionID: sug.ID,
		FilePath:     sug.RelevantFile,
		OldStatus:    sug.ImplementationStatus,
		NewStatus:    domain.ImplStatusIgnored,
	}

	// 1. Direct match: The exact suggested improvement exists in added lines
	if targetSnippet != "" && strings.Contains(addedLines, targetSnippet) {
		outcome.NewStatus = domain.ImplStatusImplemented
		outcome.MatchedRatio = 1.0
		outcome.Reason = "Target suggested code snippet was added exactly in commit diff"

		// Auto-resolve comment thread if thread ID exists
		if p.threadResolver != nil && sug.Comment != nil && sug.Comment.PlatformCommentID != "" {
			err := p.threadResolver.ResolveThread(ctx, repoID, pullNumber, sug.Comment.PlatformCommentID)
			outcome.AutoResolvedThread = (err == nil)
		}
		return outcome
	}

	// 2. Heuristic similarity match: check tokens of target code in added lines
	ratio := calculateTokenMatchRatio(targetSnippet, addedLines)
	outcome.MatchedRatio = ratio

	if ratio >= 0.70 {
		outcome.NewStatus = domain.ImplStatusImplemented
		outcome.Reason = fmt.Sprintf("High similarity (%.1f%%) to suggested replacement", ratio*100)
		if p.threadResolver != nil && sug.Comment != nil && sug.Comment.PlatformCommentID != "" {
			_ = p.threadResolver.ResolveThread(ctx, repoID, pullNumber, sug.Comment.PlatformCommentID)
			outcome.AutoResolvedThread = true
		}
	} else if ratio >= 0.35 || isOldFlawedCodeRemoved(sug.ExistingCode, deletedLines) {
		outcome.NewStatus = domain.ImplStatusPartiallyImplemented
		outcome.Reason = "Original problematic code was removed or partially refactored"
	} else {
		outcome.NewStatus = domain.ImplStatusIgnored
		outcome.Reason = "Modified file without adopting suggested code recommendation"
	}

	return outcome
}

func extractAddedLines(patch string) string {
	if patch == "" {
		return ""
	}
	var sb strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			sb.WriteString(strings.TrimPrefix(line, "+"))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func extractDeletedLines(patch string) string {
	if patch == "" {
		return ""
	}
	var sb strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			sb.WriteString(strings.TrimPrefix(line, "-"))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func isOldFlawedCodeRemoved(existingCode, deletedLines string) bool {
	cleanExisting := strings.TrimSpace(existingCode)
	if cleanExisting == "" {
		return false
	}
	return strings.Contains(deletedLines, cleanExisting)
}

func calculateTokenMatchRatio(suggested, patchAdded string) float64 {
	suggestedTokens := tokenize(suggested)
	if len(suggestedTokens) == 0 {
		return 0.0
	}

	patchTokens := tokenize(patchAdded)
	if len(patchTokens) == 0 {
		return 0.0
	}

	matched := 0
	for token := range suggestedTokens {
		if _, exists := patchTokens[token]; exists {
			matched++
		}
	}

	return float64(matched) / float64(len(suggestedTokens))
}

func tokenize(text string) map[string]struct{} {
	words := strings.Fields(strings.ToLower(text))
	tokens := make(map[string]struct{}, len(words))
	for _, w := range words {
		clean := strings.Trim(w, ".,:;()[]\"'{}<>=+-*/")
		if len(clean) > 2 {
			tokens[clean] = struct{}{}
		}
	}
	return tokens
}
