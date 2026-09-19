// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/agents/skills/capabilities/taskcontext"
	"github.com/scandrix/backend/internal/agents/skills/runtime"
)

const (
	SkillName            = "business-rules-validation"
	PrMetadataCapability = "pr.metadata.read"
	PrDiffCapability     = "pr.diff.read"
)

// BusinessRulesBlueprintTooling provides deterministic and agentic fetching of PR and task data.
type BusinessRulesBlueprintTooling interface {
	FetchPullRequestBody(ctx context.Context, bctx BusinessRulesContext) (string, error)
	FetchPullRequestDiff(ctx context.Context, bctx BusinessRulesContext) (string, error)
	FetchTaskContext(ctx context.Context, bctx BusinessRulesContext) (*TaskMetadata, error)
}

// DefaultBusinessRulesBlueprintTooling implements BusinessRulesBlueprintTooling.
type DefaultBusinessRulesBlueprintTooling struct {
	fetcher           runtime.ToolCaller
	capabilityRuntime *runtime.SkillCapabilityRuntimeConfig
	hooks             *runtime.CapabilityExecutionHooks
}

// NewDefaultBusinessRulesBlueprintTooling creates a new blueprint tooling instance.
func NewDefaultBusinessRulesBlueprintTooling(
	fetcher runtime.ToolCaller,
	config *runtime.SkillCapabilityRuntimeConfig,
	hooks *runtime.CapabilityExecutionHooks,
) *DefaultBusinessRulesBlueprintTooling {
	return &DefaultBusinessRulesBlueprintTooling{
		fetcher:           fetcher,
		capabilityRuntime: config,
		hooks:             hooks,
	}
}

// FetchPullRequestBody fetches the PR body from tool or returns PRBody from context.
func (t *DefaultBusinessRulesBlueprintTooling) FetchPullRequestBody(
	ctx context.Context,
	bctx BusinessRulesContext,
) (string, error) {
	if bctx.PRBody != "" {
		return bctx.PRBody, nil
	}

	if t.fetcher == nil {
		return "", nil
	}

	// If tool available, call it deterministically
	toolName := "getPullRequest"
	res, err := t.fetcher.CallTool(ctx, toolName, map[string]any{
		"organizationId": bctx.OrganizationID,
		"teamId":         bctx.TeamID,
		"repositoryId":   bctx.RepositoryID,
	})
	if err != nil || res == nil || res.Result == nil {
		return "", err
	}

	if m, ok := res.Result.(map[string]any); ok {
		if body, ok := m["body"].(string); ok {
			return body, nil
		}
	}

	return fmt.Sprintf("%v", res.Result), nil
}

// FetchPullRequestDiff fetches the PR diff from tool or returns PRDiff from context.
func (t *DefaultBusinessRulesBlueprintTooling) FetchPullRequestDiff(
	ctx context.Context,
	bctx BusinessRulesContext,
) (string, error) {
	if bctx.PRDiff != "" {
		return bctx.PRDiff, nil
	}

	if t.fetcher == nil {
		return "", nil
	}

	toolName := "getPullRequestDiff"
	res, err := t.fetcher.CallTool(ctx, toolName, map[string]any{
		"organizationId": bctx.OrganizationID,
		"teamId":         bctx.TeamID,
		"repositoryId":   bctx.RepositoryID,
	})
	if err != nil || res == nil || res.Result == nil {
		return "", err
	}

	if m, ok := res.Result.(map[string]any); ok {
		if diff, ok := m["diff"].(string); ok {
			return diff, nil
		}
	}

	return fmt.Sprintf("%v", res.Result), nil
}

// FetchTaskContext fetches task context using taskcontext extraction tools.
func (t *DefaultBusinessRulesBlueprintTooling) FetchTaskContext(
	ctx context.Context,
	bctx BusinessRulesContext,
) (*TaskMetadata, error) {
	if bctx.TaskContextNormalized != nil {
		return bctx.TaskContextNormalized, nil
	}

	if bctx.TaskContext != "" {
		meta := resolveTaskMetadata(bctx)
		return &meta, nil
	}

	if t.fetcher == nil {
		return nil, nil
	}

	// Try calling task read tool
	candidates := []string{"getJiraIssue", "getLinearIssue", "getClickUpTask", "SCANDRIX_GET_ISSUE"}
	for _, cand := range candidates {
		res, err := t.fetcher.CallTool(ctx, cand, map[string]any{
			"organizationId": bctx.OrganizationID,
			"teamId":         bctx.TeamID,
		})
		if err == nil && res != nil && res.Result != nil {
			normalized := taskcontext.ExtractTaskContextFromToolResult(res.Result)
			if normalized != nil {
				return &TaskMetadata{
					ID:                 normalized.ID,
					Title:              normalized.Title,
					Description:        normalized.Description,
					Links:              normalized.Links,
					AcceptanceCriteria: normalized.AcceptanceCriteria,
				}, nil
			}
		}
	}

	return nil, nil
}

// ResolvePullRequestDescription extracts description from context.
func ResolvePullRequestDescription(ctx BusinessRulesContext) string {
	return ctx.PRBody
}

// ResolveTaskContext extracts raw task context from context.
func ResolveTaskContext(ctx BusinessRulesContext) string {
	return ctx.TaskContext
}

// ClassifyTaskQuality returns the TaskQuality classification for a given task context.
func ClassifyTaskQuality(taskCtx string) TaskQuality {
	quality, _ := ClassifyTaskQualityFromSources(taskCtx, "")
	return quality
}
