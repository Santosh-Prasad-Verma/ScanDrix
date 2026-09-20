// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package specialists

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/review/orchestrator"
)

var (
	// TicketKeyRegex matches Jira, Linear, and project tracker keys (e.g. PROJ-123, SCANDRIX-42)
	TicketKeyRegex = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9_]+-\d+)\b`)
	// GitIssueRegex matches standard git issue numbers (e.g. #123)
	GitIssueRegex = regexp.MustCompile(`(?:^|\s)#(\d+)\b`)
)

// RequirementKeywordList contains markers indicating explicit business requirements in PR descriptions.
var RequirementKeywordList = []string{
	"requirement",
	"acceptance criteria",
	"user story",
	"given",
	"when",
	"then",
	"expected behavior",
	"spec:",
}

// BusinessLogicSpecialistAgent validates code changes against declared business requirements and tickets.
type BusinessLogicSpecialistAgent struct {
	llm LLMClient
}

// NewBusinessLogicSpecialistAgent constructs a new business requirements verification persona.
func NewBusinessLogicSpecialistAgent(llm LLMClient) *BusinessLogicSpecialistAgent {
	return &BusinessLogicSpecialistAgent{llm: llm}
}

func (a *BusinessLogicSpecialistAgent) Name() string {
	return "business_logic"
}

func (a *BusinessLogicSpecialistAgent) Category() string {
	return "business_logic"
}

// ExtractTicketKeys scans text for Jira/Linear/Git project tracker keys.
func (a *BusinessLogicSpecialistAgent) ExtractTicketKeys(text string) []string {
	var keys []string
	seen := make(map[string]struct{})

	for _, m := range TicketKeyRegex.FindAllString(text, -1) {
		upper := strings.ToUpper(m)
		if _, exists := seen[upper]; !exists {
			seen[upper] = struct{}{}
			keys = append(keys, upper)
		}
	}

	for _, m := range GitIssueRegex.FindAllStringSubmatch(text, -1) {
		if len(m) > 1 {
			k := "#" + m[1]
			if _, exists := seen[k]; !exists {
				seen[k] = struct{}{}
				keys = append(keys, k)
			}
		}
	}

	return keys
}

// HasRequirementsDeclared checks whether the PR description declares explicit acceptance criteria.
func (a *BusinessLogicSpecialistAgent) HasRequirementsDeclared(description string) bool {
	lower := strings.ToLower(description)
	for _, kw := range RequirementKeywordList {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (a *BusinessLogicSpecialistAgent) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	startTime := time.Now()

	ticketKeys := a.ExtractTicketKeys(input.Title + " " + input.Description)
	hasReqs := a.HasRequirementsDeclared(input.Description)

	// If no business requirements, tickets, or acceptance criteria are declared, complete cleanly
	if len(ticketKeys) == 0 && !hasReqs {
		return &orchestrator.ReviewAgentOutput{
			AgentName:    a.Name(),
			Category:     a.Category(),
			FinishReason: "completed",
			DurationMs:   time.Since(startTime).Milliseconds(),
		}, nil
	}

	if a.llm == nil {
		return nil, fmt.Errorf("llm client is not configured for business logic specialist")
	}

	sysPrompt := a.systemPrompt(ticketKeys)
	userPrompt := BuildSpecialistUserPrompt(input, "Business Requirements, Acceptance Criteria, and Ticket Alignment")

	raw, err := a.llm.GenerateResponse(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("business logic specialist execution failed: %w", err)
	}

	findings, err := ParseSpecialistResponse(raw, a.Name(), a.Category())
	if err != nil {
		return nil, err
	}

	return &orchestrator.ReviewAgentOutput{
		AgentName:      a.Name(),
		Category:       a.Category(),
		Findings:       findings,
		TotalTurns:     1,
		DurationMs:     time.Since(startTime).Milliseconds(),
		FinishReason:   "completed",
		TokensConsumed: len(raw) / 4,
	}, nil
}

func (a *BusinessLogicSpecialistAgent) systemPrompt(ticketKeys []string) string {
	keysStr := strings.Join(ticketKeys, ", ")
	if keysStr == "" {
		keysStr = "None explicitly declared"
	}

	return fmt.Sprintf(`You are ScanDrix AI's Principal Product & Business Logic Auditor.
Your role is to verify that the implementation matches the business requirements and acceptance criteria declared in the PR description and linked tickets (%s).

FOCUS AREAS:
- Acceptance Criteria Discrepancies (code fails to implement a required condition)
- Missed Edge Cases in Business Flow (missing error states, unhandled discount edge cases, negative values)
- Inverted Business Logic (authorizing when should deny, charging inverted amounts)
- Missing Validation for Domain Requirements (e.g. required form fields, boundary conditions)
- Accidental Scope Deletion (code changes inadvertently removing existing business features)

OUTPUT FORMAT (strictly valid JSON):
{
  "findings": [
    {
      "file_path": "path/to/file.go",
      "start_line": 80,
      "end_line": 85,
      "severity": "HIGH",
      "confidence": "HIGH",
      "category": "business_logic",
      "title": "Acceptance Criteria Violation: Missing Grace Period Calculation",
      "description": "PR description states 'subscriptions must enter a 3-day grace period upon payment failure', but the implementation immediately terminates the subscription.",
      "remediation": "Check if grace period has expired before terminating: if time.Since(failedAt) < 3*24*time.Hour { enterGracePeriod() }",
      "improved_code": "if time.Since(sub.PaymentFailedAt) < 72*time.Hour {\n    sub.Status = StatusGracePeriod\n}",
      "blocking": true
    }
  ]
}
If the code correctly satisfies all declared business requirements, return {"findings": []}.`, keysStr)
}
