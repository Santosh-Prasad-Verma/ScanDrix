// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_comment_analysis_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/domain/prompts"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

const (
	SubstantiveCommentLengthThreshold = 100
	MaxCandidateRulesPerRun           = 12
)

// DrixyCommentAnalysisService coordinates filtering, synthesis, and deduplication of review rules.
type DrixyCommentAnalysisService struct {
	llmGateway   *llm.Gateway
	rulesService contracts.IDrixyRulesService
}

// NewDrixyCommentAnalysisService constructs a new comment analysis and rule synthesis service.
func NewDrixyCommentAnalysisService(gw *llm.Gateway, rs contracts.IDrixyRulesService) *DrixyCommentAnalysisService {
	return &DrixyCommentAnalysisService{
		llmGateway:   gw,
		rulesService: rs,
	}
}

// ProcessComments filters out trivial comments, bot messages, and excluded reviewers.
func (s *DrixyCommentAnalysisService) ProcessComments(
	rawComments []prompts.ReviewCommentInput,
	excludedReviewers map[string]bool,
) []prompts.ReviewCommentInput {
	var processed []prompts.ReviewCommentInput
	for _, c := range rawComments {
		body := strings.TrimSpace(c.Body)
		if len(body) < SubstantiveCommentLengthThreshold {
			continue
		}
		// Exclude bot and automated signals
		lower := strings.ToLower(body)
		if strings.Contains(lower, "[bot]") ||
			strings.Contains(lower, "github-actions") ||
			strings.Contains(lower, "codecov") ||
			strings.Contains(lower, "sonar") ||
			strings.Contains(lower, "automated test") {
			continue
		}
		if c.ID != "" && excludedReviewers != nil && excludedReviewers[c.ID] {
			continue
		}
		processed = append(processed, c)
	}
	return processed
}

// GenerateRulesFromComments satisfies the ICommentAnalysisService interface in usecases.
func (s *DrixyCommentAnalysisService) GenerateRulesFromComments(
	ctx context.Context,
	comments []string,
) ([]dtos.CreateDrixyRuleDto, error) {
	var inputs []prompts.ReviewCommentInput
	for i, c := range comments {
		inputs = append(inputs, prompts.ReviewCommentInput{
			ID:       fmt.Sprintf("comment-%d", i+1),
			Body:     c,
			Language: "any",
		})
	}
	return s.SynthesizeRules(ctx, inputs, nil)
}

// SynthesizeRules executes the multi-stage LLM pipeline: synthesis -> duplicate filtering -> quality gating.
func (s *DrixyCommentAnalysisService) SynthesizeRules(
	ctx context.Context,
	comments []prompts.ReviewCommentInput,
	existingRules []interfaces.DrixyRule,
) ([]dtos.CreateDrixyRuleDto, error) {
	if s.llmGateway == nil || len(comments) == 0 {
		return nil, nil
	}

	// 1. Synthesize candidate rules
	userPrompt := prompts.DrixyRulesGeneratorUser(comments, nil)
	resp, err := s.llmGateway.GenerateChatResponse(ctx, prompts.DrixyRulesGeneratorSystem(), nil, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed generating rules from LLM: %w", err)
	}

	cleanJSON := cleanJSONBlock(resp)
	var genOutput prompts.RuleGeneratorOutput
	if err := json.Unmarshal([]byte(cleanJSON), &genOutput); err != nil {
		return nil, fmt.Errorf("failed parsing generated rules JSON: %w", err)
	}

	if len(genOutput.Rules) == 0 {
		return nil, nil
	}

	// Assign temporary UUIDs for filtering stages
	var candidates []interfaces.DrixyRule
	ruleMap := make(map[string]prompts.GeneratedRuleItem)
	for _, r := range genOutput.Rules {
		tempID := r.UUID
		if tempID == "" {
			tempID = uuid.New().String()
		}
		ruleMap[tempID] = r

		candidates = append(candidates, interfaces.DrixyRule{
			UUID:     tempID,
			Title:    r.Title,
			Rule:     r.Rule,
			Severity: r.Severity,
			Examples: r.Examples,
		})
	}

	// 2. Duplicate filtering against existing rules
	survivingCandidates := candidates
	if len(existingRules) > 0 && len(candidates) > 0 {
		dupPrompt := prompts.DrixyRulesDuplicateFilterUser(existingRules, candidates)
		dupResp, err := s.llmGateway.GenerateChatResponse(ctx, prompts.DrixyRulesDuplicateFilterSystem(), nil, dupPrompt)
		if err == nil {
			var dupOutput prompts.UUIDListOutput
			if err := json.Unmarshal([]byte(cleanJSONBlock(dupResp)), &dupOutput); err == nil && len(dupOutput.UUIDs) > 0 {
				idSet := make(map[string]bool)
				for _, id := range dupOutput.UUIDs {
					idSet[id] = true
				}
				var filtered []interfaces.DrixyRule
				for _, cand := range candidates {
					if idSet[cand.UUID] {
						filtered = append(filtered, cand)
					}
				}
				if len(filtered) > 0 {
					survivingCandidates = filtered
				}
			}
		}
	}

	// 3. Quality filtering
	finalCandidates := survivingCandidates
	if len(survivingCandidates) > 0 {
		qualPrompt := prompts.DrixyRulesQualityFilterUser(survivingCandidates)
		qualResp, err := s.llmGateway.GenerateChatResponse(ctx, prompts.DrixyRulesQualityFilterSystem(), nil, qualPrompt)
		if err == nil {
			var qualOutput prompts.UUIDListOutput
			if err := json.Unmarshal([]byte(cleanJSONBlock(qualResp)), &qualOutput); err == nil && len(qualOutput.UUIDs) > 0 {
				idSet := make(map[string]bool)
				for _, id := range qualOutput.UUIDs {
					idSet[id] = true
				}
				var filtered []interfaces.DrixyRule
				for _, cand := range survivingCandidates {
					if idSet[cand.UUID] {
						filtered = append(filtered, cand)
					}
				}
				if len(filtered) > 0 {
					finalCandidates = filtered
				}
			}
		}
	}

	// 4. Map final candidates to DTOs
	var dtosList []dtos.CreateDrixyRuleDto
	for _, cand := range finalCandidates {
		orig := ruleMap[cand.UUID]
		sev := orig.Severity
		if sev == "" {
			sev = "MEDIUM"
		}
		scope := interfaces.DrixyRulesScopeFile
		if strings.EqualFold(orig.Scope, "pull-request") || strings.EqualFold(orig.Scope, "pull_request") {
			scope = interfaces.DrixyRulesScopePullRequest
		}

		dto := dtos.CreateDrixyRuleDto{
			Title:    orig.Title,
			Rule:     orig.Rule,
			Severity: sev,
			Path:     "*/**",
			Scope:    scope,
			Origin:   interfaces.DrixyRulesOriginPastReviews,
			Status:   interfaces.DrixyRulesStatusActive,
			Type:     interfaces.DrixyRulesTypeStandard,
			Examples: make([]interfaces.DrixyRulesExample, 0),
		}

		for _, ex := range orig.Examples {
			dto.Examples = append(dto.Examples, interfaces.DrixyRulesExample{
				Snippet:   ex.Snippet,
				IsCorrect: ex.IsCorrect,
			})
		}

		dtosList = append(dtosList, dto)
		if len(dtosList) >= MaxCandidateRulesPerRun {
			break
		}
	}

	return dtosList, nil
}
