// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/domain"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

const (
	defaultBusinessRulesMaxSteps = 5
	defaultBusinessRulesTokens   = 4000
)

const systemPrompt = `You are ScanDrix AI's Business Rules Validation Agent.
Your job is to perform a rigorous gap analysis comparing the Pull Request diff against the User Story, Jira/Linear task requirements, and Acceptance Criteria.

Your output must be a valid JSON object adhering to this schema:
{
  "is_compliant": true|false,
  "needs_more_info": true|false,
  "summary": "High-level assessment in user language",
  "missing_requirements": ["list of unsatisfied acceptance criteria"],
  "violated_rules": ["list of violated architectural or domain business rules"],
  "suggestions": ["remediation steps for the engineer"],
  "confidence": "high"|"medium"|"low"
}

Grounding Rules:
- If the task context is missing or too vague to judge the diff, set needs_more_info: true.
- Do not hallucinate requirements not present in the task description.
- Return ONLY the JSON object, with no conversational filler.`

// TaskFetcher retrieves external task/issue requirements from project management providers (Jira, Linear, GitHub).
type TaskFetcher interface {
	FetchTaskContext(ctx context.Context, taskKeys []string) (string, error)
}

// BusinessRulesAgentOptions configures limits, verification toggles, and external task fetchers.
type BusinessRulesAgentOptions struct {
	VerifyEnabled   bool
	MaxSteps        int
	MaxOutputTokens int
	TaskFetcher     TaskFetcher
}

// WithTaskFetcher returns a BusinessRulesAgentOptions configured with the given TaskFetcher.
func WithTaskFetcher(fetcher TaskFetcher) BusinessRulesAgentOptions {
	return BusinessRulesAgentOptions{
		TaskFetcher: fetcher,
	}
}

// BusinessRulesValidationAgentProvider orchestrates business logic gap analysis.
type BusinessRulesValidationAgentProvider struct {
	runner   contracts.AgentRunner
	verifier *BusinessRulesVerifier
	opts     BusinessRulesAgentOptions
}

// NewBusinessRulesValidationAgentProvider constructs the agent provider.
// Automatically reads SCANDRIX_BUSINESS_RULES_VERIFY_ENABLED and limits from environment.
func NewBusinessRulesValidationAgentProvider(
	runner contracts.AgentRunner,
	customOpts ...BusinessRulesAgentOptions,
) *BusinessRulesValidationAgentProvider {
	opts := BusinessRulesAgentOptions{
		VerifyEnabled:   true,
		MaxSteps:        defaultBusinessRulesMaxSteps,
		MaxOutputTokens: defaultBusinessRulesTokens,
	}

	if envVerify := os.Getenv("SCANDRIX_BUSINESS_RULES_VERIFY_ENABLED"); envVerify != "" {
		if parsed, err := strconv.ParseBool(envVerify); err == nil {
			opts.VerifyEnabled = parsed
		}
	}
	if envSteps := os.Getenv("SCANDRIX_BUSINESS_RULES_MAX_STEPS"); envSteps != "" {
		if parsed, err := strconv.Atoi(envSteps); err == nil && parsed > 0 {
			opts.MaxSteps = parsed
		}
	}
	if envTokens := os.Getenv("SCANDRIX_BUSINESS_RULES_MAX_TOKENS"); envTokens != "" {
		if parsed, err := strconv.Atoi(envTokens); err == nil && parsed > 0 {
			opts.MaxOutputTokens = parsed
		}
	}

	if len(customOpts) > 0 {
		userOpt := customOpts[0]
		opts.VerifyEnabled = userOpt.VerifyEnabled
		if userOpt.MaxSteps > 0 {
			opts.MaxSteps = userOpt.MaxSteps
		}
		if userOpt.MaxOutputTokens > 0 {
			opts.MaxOutputTokens = userOpt.MaxOutputTokens
		}
		if userOpt.TaskFetcher != nil {
			opts.TaskFetcher = userOpt.TaskFetcher
		}
	}

	return &BusinessRulesValidationAgentProvider{
		runner:   runner,
		verifier: NewBusinessRulesVerifier(runner),
		opts:     opts,
	}
}

// Execute performs gap analysis and optional dual-pass verification.
func (p *BusinessRulesValidationAgentProvider) Execute(
	ctx context.Context,
	bctx BusinessRulesContext,
) (*ValidationResult, error) {
	// 0. Auto-resolve task context from PR diff and body if empty
	if strings.TrimSpace(bctx.TaskContext) == "" && p.opts.TaskFetcher != nil {
		keys := ExtractTaskIdentifiers(bctx.PRDiff, bctx.PRBody)
		if len(keys) > 0 {
			if fetched, err := p.opts.TaskFetcher.FetchTaskContext(ctx, keys); err == nil && strings.TrimSpace(fetched) != "" {
				bctx.TaskContext = fetched
			}
		}
	}

	// 1. Guard against empty input and evaluate pre-flight eligibility
	if bctx.TaskQuality == "" {
		quality, meta := ClassifyTaskQualityFromSources(bctx.TaskContext, bctx.PRBody)
		bctx.TaskQuality = string(quality)
		if bctx.TaskContextNormalized == nil {
			bctx.TaskContextNormalized = meta
		}
	}

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

	prompt := BuildBusinessRulesAnalysisPrompt(bctx)
	maxTokens := p.opts.MaxOutputTokens

	spec := contracts.AgentSpec{
		ID:              "business-rules-validation",
		AgentName:       "BusinessRulesValidation",
		RunName:         "businessRulesValidationAgent",
		Phase:           "businessRulesValidation",
		SpanName:        "BusinessRulesValidation::validate",
		SystemPrompt:    systemPrompt,
		Tools:           tools.NewInMemoryToolRegistry(),
		MaxSteps:        p.opts.MaxSteps,
		MaxOutputTokens: &maxTokens,
	}

	toolCtx := contracts.ToolContext{
		RunID:   fmt.Sprintf("brv-%s-%d", bctx.OrganizationID, time.Now().UnixNano()),
		Context: ctx,
		Services: map[string]any{
			"organization_id": bctx.OrganizationID,
			"team_id":         bctx.TeamID,
		},
	}

	runInput := contracts.AgentRunInput{
		Prompt: prompt,
		TelemetryMetadata: map[string]any{
			"organization_id": bctx.OrganizationID,
			"team_id":         bctx.TeamID,
			"thread_id":       bctx.ThreadID,
			"repository_id":   bctx.RepositoryID,
		},
	}

	// 1. Run Analyzer pass
	state, err := p.runner.Run(ctx, spec, runInput, toolCtx)
	if err != nil {
		slog.Error("BusinessRulesValidationAgent execution failed", "error", err)
		res := buildFallbackResult(fmt.Sprintf("execution error: %v", err))
		return &res, nil
	}

	rawAnswer := domain.FinalText(state)
	parsedResult := ParseValidationResult(rawAnswer)

	// 2. Optional Dual-Pass Verification (doer != checker)
	if ShouldVerifyValidationResult(parsedResult, p.opts.VerifyEnabled) {
		slog.Info("Running business rules verification pass to refute false positives",
			"organization_id", bctx.OrganizationID,
			"violations_count", len(parsedResult.ViolatedRules)+len(parsedResult.MissingRequirements),
		)
		verdict, verifyErr := p.verifier.Verify(ctx, parsedResult, bctx)
		if verifyErr == nil {
			parsedResult = ApplyBusinessRulesVerdict(parsedResult, verdict)
		}
	}

	return &parsedResult, nil
}

// FormatBusinessRulesPRComment converts a ValidationResult into a clean markdown comment
// suitable for posting onto a pull request review.
func FormatBusinessRulesPRComment(result *ValidationResult, prTitle string) string {
	if result == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 🛡️ ScanDrix Business Rules Validation\n\n")

	if prTitle != "" {
		sb.WriteString(fmt.Sprintf("**PR**: %s\n", prTitle))
	}

	if result.NeedsMoreInfo {
		sb.WriteString("**Status**: ⚠️ Needs More Information\n\n")
		if result.MissingInfo != "" {
			sb.WriteString(result.MissingInfo)
		} else if result.Summary != "" {
			sb.WriteString(result.Summary)
		} else {
			sb.WriteString("Could not validate business rules because task context is insufficient. Please link a Jira/Linear ticket or provide requirements in the PR description.")
		}
		return strings.TrimSpace(sb.String())
	}

	if result.IsCompliant {
		sb.WriteString("**Status**: ✅ Compliant\n")
		if result.Confidence != "" {
			sb.WriteString(fmt.Sprintf("**Confidence**: %s\n", result.Confidence))
		}
		sb.WriteString("\n")
		if result.Summary != "" {
			sb.WriteString(result.Summary)
			sb.WriteString("\n\n")
		}
		if len(result.Suggestions) > 0 {
			sb.WriteString("### 💡 Suggestions\n")
			for _, sug := range result.Suggestions {
				sb.WriteString(fmt.Sprintf("- %s\n", sug))
			}
		}
		return strings.TrimSpace(sb.String())
	}

	// Non-compliant
	sb.WriteString("**Status**: ❌ Issues Found\n")
	if result.Confidence != "" {
		sb.WriteString(fmt.Sprintf("**Confidence**: %s\n", result.Confidence))
	}
	sb.WriteString("\n")

	if result.Summary != "" {
		sb.WriteString(result.Summary)
		sb.WriteString("\n\n")
	}

	if len(result.ViolatedRules) > 0 {
		sb.WriteString("### 🚨 Violated Rules\n")
		for _, v := range result.ViolatedRules {
			sb.WriteString(fmt.Sprintf("- %s\n", v))
		}
		sb.WriteString("\n")
	}

	if len(result.MissingRequirements) > 0 {
		sb.WriteString("### 📋 Missing Requirements\n")
		for _, m := range result.MissingRequirements {
			sb.WriteString(fmt.Sprintf("- %s\n", m))
		}
		sb.WriteString("\n")
	}

	if len(result.Suggestions) > 0 {
		sb.WriteString("### 💡 Suggestions\n")
		for _, sug := range result.Suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", sug))
		}
	}

	return strings.TrimSpace(sb.String())
}
