// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"regexp"
	"strings"
)

var (
	jiraOrLinearKeyRegex = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)
	githubIssueKeyRegex  = regexp.MustCompile(`(?i)(?:close[sd]?|fixe?[sd]?|resolve[sd]?|issue)\s*#(\d+)`)
	urlIssueRegex        = regexp.MustCompile(`https?://[^\s)]+/(?:browse/|issues/|issue/)?([A-Z][A-Z0-9]+-\d+|\d+)`)
)

// ExtractTaskIdentifiers finds task and ticket identifiers (Jira, Linear, GitHub) from arbitrary text.
func ExtractTaskIdentifiers(texts ...string) []string {
	seen := make(map[string]bool)
	var keys []string

	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			continue
		}

		// 1. Jira / Linear keys: PROJ-123
		for _, match := range jiraOrLinearKeyRegex.FindAllStringSubmatch(text, -1) {
			key := match[1]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}

		// 2. GitHub issue numbers: #123
		for _, match := range githubIssueKeyRegex.FindAllStringSubmatch(text, -1) {
			key := "#" + match[1]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}

		// 3. URLs pointing to issues
		for _, match := range urlIssueRegex.FindAllStringSubmatch(text, -1) {
			key := match[1]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// ClassifyTaskQualityFromSources evaluates quality across both task context and pull request body.
func ClassifyTaskQualityFromSources(taskContext string, prBody string) (TaskQuality, *TaskMetadata) {
	meta := resolveTaskMetadata(BusinessRulesContext{
		TaskContext: taskContext,
		PRBody:      prBody,
	})

	trimmedCtx := strings.TrimSpace(taskContext)
	if trimmedCtx == "" {
		// Check if PR body has structured acceptance criteria
		if len(meta.AcceptanceCriteria) > 0 {
			return TaskQualityPartial, &meta
		}
		return TaskQualityEmpty, &meta
	}

	if looksLikeStructuredMetadata(trimmedCtx) || looksLikeTaskFetchFailure(trimmedCtx) {
		return TaskQualityMinimal, &meta
	}

	// If description is comprehensive and has explicit criteria
	if len(meta.AcceptanceCriteria) > 0 && len(meta.Description) > 60 {
		return TaskQualityComplete, &meta
	}

	if len(meta.Description) > 30 || len(meta.AcceptanceCriteria) > 0 {
		return TaskQualityPartial, &meta
	}

	return TaskQualityMinimal, &meta
}

// BlueprintPipeline coordinates task context extraction, eligibility classification, and agent execution.
type BlueprintPipeline struct {
	provider *BusinessRulesValidationAgentProvider
}

// NewBlueprintPipeline constructs the multi-step pipeline.
func NewBlueprintPipeline(provider *BusinessRulesValidationAgentProvider) *BlueprintPipeline {
	return &BlueprintPipeline{provider: provider}
}

// Run executes the complete verification pipeline.
func (b *BlueprintPipeline) Run(ctx context.Context, bctx BusinessRulesContext) (*ValidationResult, error) {
	if b.provider == nil {
		res := buildFallbackResult("business rules validation provider unavailable")
		return &res, nil
	}

	// Auto-classify task quality if not explicitly supplied
	if bctx.TaskQuality == "" {
		quality, meta := ClassifyTaskQualityFromSources(bctx.TaskContext, bctx.PRBody)
		bctx.TaskQuality = string(quality)
		if bctx.TaskContextNormalized == nil {
			bctx.TaskContextNormalized = meta
		}
	}

	// Pre-flight eligibility evaluation
	quality := NormalizeTaskQuality(bctx.TaskQuality)
	eligibility := BuildBusinessLogicEligibility(quality, bctx.TaskContext, bctx.PRDiff, bctx.TaskContextNormalized)

	if eligibility.Mode == EligibilityModeLimitationResponse {
		var missingInfoMsg string
		if eligibility.TaskContextStatus != TaskContextStatusUsable {
			missingInfoMsg = GetTaskContextMissingInfoMessage(quality)
		} else {
			missingInfoMsg = GetPullRequestDiffMissingInfoMessage()
		}

		return &ValidationResult{
			NeedsMoreInfo: true,
			IsCompliant:   false,
			Mode:          string(eligibility.Mode),
			Reason:        eligibility.Reason,
			Summary:       "Business rules validation cannot proceed with missing or insufficient requirements context.",
			MissingInfo:   missingInfoMsg,
			Confidence:    "high",
		}, nil
	}

	// Eligible for full analysis
	return b.provider.Execute(ctx, bctx)
}
