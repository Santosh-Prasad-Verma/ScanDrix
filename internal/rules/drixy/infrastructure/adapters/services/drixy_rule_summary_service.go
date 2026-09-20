// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rule_summary_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

const (
	LongRuleThresholdChars = 1000
	MaxAtomsPerRule        = 12
)

// DrixyRuleSummaryService handles structured summaries and atomic decompositions of complex rules.
type DrixyRuleSummaryService struct {
	llmGateway *llm.Gateway
}

// NewDrixyRuleSummaryService initializes the summary service.
func NewDrixyRuleSummaryService(gw *llm.Gateway) *DrixyRuleSummaryService {
	return &DrixyRuleSummaryService{llmGateway: gw}
}

// ComputeSourceHash generates a deterministic SHA-256 hash for rule validation guardrails.
func ComputeSourceHash(ruleText string, examples []interfaces.DrixyRulesExample) string {
	h := sha256.New()
	h.Write([]byte(ruleText))
	if len(examples) > 0 {
		exBytes, _ := json.Marshal(examples)
		h.Write(exBytes)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateSummary extracts structured validation bullets for long rules (>1000 characters).
func (s *DrixyRuleSummaryService) GenerateSummary(ctx context.Context, rule *interfaces.DrixyRule) (*interfaces.DrixyRuleSummary, error) {
	if rule == nil || len(rule.Rule) < LongRuleThresholdChars {
		return nil, nil
	}

	sourceHash := ComputeSourceHash(rule.Rule, nil)

	// If LLM gateway is available, generate structured validation bullets
	if s.llmGateway != nil {
		prompt := fmt.Sprintf(
			"You are an expert code reviewer assistant. Summarize the following complex review rule into structured bullets with WHAT TO VALIDATE and HOW TO VALIDATE in code diffs:\n\nTitle: %s\nRule:\n%s",
			rule.Title, rule.Rule,
		)
		respText, err := s.llmGateway.GenerateChatResponse(ctx, "You are an expert code reviewer assistant.", nil, prompt)
		if err == nil && strings.TrimSpace(respText) != "" {
			return &interfaces.DrixyRuleSummary{
				Content:     strings.TrimSpace(respText),
				SourceHash:  sourceHash,
				GeneratedAt: time.Now().UTC(),
				Model:       "scandrix-ai-gateway",
			}, nil
		}
	}

	// High-fidelity fallback heuristic summary
	var summaryBuilder strings.Builder
	summaryBuilder.WriteString("WHAT TO VALIDATE:\n")
	lines := strings.Split(rule.Rule, "\n")
	count := 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && len(trimmed) > 15 && count < 3 {
			summaryBuilder.WriteString(fmt.Sprintf("- %s\n", trimmed))
			count++
		}
	}
	summaryBuilder.WriteString("\nHOW TO VALIDATE:\n- Verify added and modified diff lines comply with required patterns and architectural constraints.")

	return &interfaces.DrixyRuleSummary{
		Content:     summaryBuilder.String(),
		SourceHash:  sourceHash,
		GeneratedAt: time.Now().UTC(),
		Model:       "scandrix-deterministic-heuristic",
	}, nil
}

// ResolveForReview swaps the rule text with its structured summary if the rule is long and the summary matches sourceHash.
func (s *DrixyRuleSummaryService) ResolveForReview(rule *interfaces.DrixyRule) *interfaces.DrixyRule {
	if rule == nil || len(rule.Rule) < LongRuleThresholdChars || rule.Summary == nil || rule.Summary.Content == "" {
		return rule
	}
	currentHash := ComputeSourceHash(rule.Rule, nil)
	if rule.Summary.SourceHash != currentHash {
		// Summary is stale, return untouched full text
		return rule
	}
	cp := *rule
	cp.Rule = rule.Summary.Content
	return &cp
}

// DecomposeRule splits compound rules into atomic requirements.
func (s *DrixyRuleSummaryService) DecomposeRule(ctx context.Context, rule *interfaces.DrixyRule) (*interfaces.DrixyRuleAtoms, error) {
	if rule == nil || len(rule.Rule) < LongRuleThresholdChars {
		return nil, nil
	}

	sourceHash := ComputeSourceHash(rule.Rule, rule.Examples)

	if s.llmGateway != nil {
		prompt := fmt.Sprintf(
			"You decompose a long team code-review rule into ATOMIC requirements. Each atom is ONE independently-checkable condition a reviewer flags in a code diff.\n\nReturn ONLY JSON:\n{\"atoms\":[{\"title\":\"<short imperative label, <=80 chars>\",\"spec\":\"WHAT: <condition>\\nHOW: <pattern in diff>\"}]}\n\nRule:\n%s",
			rule.Rule,
		)
		respText, err := s.llmGateway.GenerateChatResponse(ctx, "You are an expert code analyzer.", nil, prompt)
		if err == nil {
			var parsed struct {
				Atoms []struct {
					Title string `json:"title"`
					Spec  string `json:"spec"`
				} `json:"atoms"`
			}
			text := cleanJSONBlock(respText)
			if err := json.Unmarshal([]byte(text), &parsed); err == nil && len(parsed.Atoms) > 0 {
				var items []interfaces.DrixyRuleAtom
				for i, a := range parsed.Atoms {
					if i >= MaxAtomsPerRule {
						break
					}
					items = append(items, interfaces.DrixyRuleAtom{
						ID:    fmt.Sprintf("%s-atom-%d", rule.UUID, i+1),
						Title: a.Title,
						Spec:  a.Spec,
					})
				}
				return &interfaces.DrixyRuleAtoms{
					Items:       items,
					SourceHash:  sourceHash,
					GeneratedAt: time.Now().UTC(),
					Model:       "scandrix-ai-gateway",
				}, nil
			}
		}
	}

	// Heuristic decomposition fallback
	var items []interfaces.DrixyRuleAtom
	paragraphs := strings.Split(rule.Rule, "\n\n")
	idx := 1
	for _, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if len(trimmed) > 30 && idx <= MaxAtomsPerRule {
			title := trimmed
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			items = append(items, interfaces.DrixyRuleAtom{
				ID:    fmt.Sprintf("%s-atom-%d", rule.UUID, idx),
				Title: title,
				Spec:  fmt.Sprintf("WHAT: %s\nHOW: Inspect modified diff lines for violations.", trimmed),
			})
			idx++
		}
	}

	return &interfaces.DrixyRuleAtoms{
		Items:       items,
		SourceHash:  sourceHash,
		GeneratedAt: time.Now().UTC(),
		Model:       "scandrix-deterministic-heuristic",
	}, nil
}

func cleanJSONBlock(input string) string {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			if strings.HasPrefix(lines[0], "```") {
				lines = lines[1:]
			}
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			return strings.Join(lines, "\n")
		}
	}
	return trimmed
}
