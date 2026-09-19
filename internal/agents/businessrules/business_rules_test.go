// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Business Rules Validation Agent
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package businessrules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agents/skills/runtime"
)

type mockAgentRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestCriteriaExtraction(t *testing.T) {
	taskText := `
Task: AUTH-101
Title: Add MFA Authentication

Requirements:
- [ ] User must be able to register TOTP authenticator
- [ ] Backup codes must be generated on activation
- [x] Rate limit verification attempts to 5 per minute
* Email notification sent when MFA enabled
`
	criteria := ExtractCriteriaFromText(taskText)
	if len(criteria) != 4 {
		t.Fatalf("expected 4 criteria extracted, got %d: %v", len(criteria), criteria)
	}

	taskID := ExtractTaskIDFromText(taskText)
	if taskID != "AUTH-101" {
		t.Errorf("expected AUTH-101, got %s", taskID)
	}

	taskTitle := ExtractTaskTitleFromText(taskText)
	if taskTitle != "Add MFA Authentication" {
		t.Errorf("expected Add MFA Authentication, got %s", taskTitle)
	}
}

func TestParseValidationResult(t *testing.T) {
	cases := []struct {
		name        string
		raw         string
		isCompliant bool
		needsInfo   bool
	}{
		{
			name: "Compliant JSON",
			raw: `{
				"is_compliant": true,
				"needs_more_info": false,
				"summary": "All MFA requirements implemented.",
				"missing_requirements": []
			}`,
			isCompliant: true,
			needsInfo:   false,
		},
		{
			name: "Fenced Non-Compliant JSON",
			raw: "```json\n" + `{
				"is_compliant": false,
				"needs_more_info": false,
				"summary": "Missing backup codes.",
				"missing_requirements": ["Backup codes must be generated on activation"]
			}` + "\n```",
			isCompliant: false,
			needsInfo:   false,
		},
		{
			name:        "Limitation response",
			raw:         "Need task information before evaluating PR diff.",
			isCompliant: false,
			needsInfo:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ParseValidationResult(tc.raw)
			if res.IsCompliant != tc.isCompliant {
				t.Errorf("expected isCompliant=%v, got %v", tc.isCompliant, res.IsCompliant)
			}
			if res.NeedsMoreInfo != tc.needsInfo {
				t.Errorf("expected needsMoreInfo=%v, got %v", tc.needsInfo, res.NeedsMoreInfo)
			}
		})
	}
}

func TestApplyBusinessRulesVerdict_RefuteToDrop(t *testing.T) {
	initial := ValidationResult{
		IsCompliant:         false,
		NeedsMoreInfo:       false,
		Summary:             "Analyzer claims missing backup codes.",
		MissingRequirements: []string{"Backup codes required"},
	}

	// 1. Keep = true (Trust analyzer)
	kept := ApplyBusinessRulesVerdict(initial, Verdict{Keep: true, Rationale: "Violation genuinely holds"})
	if kept.IsCompliant {
		t.Errorf("expected kept result to stay non-compliant")
	}

	// 2. Keep = false (Refuted by verifier)
	refuted := ApplyBusinessRulesVerdict(initial, Verdict{
		Keep:      false,
		Rationale: "Diff lines 40-60 implement backup code generator.",
	})
	if !refuted.IsCompliant {
		t.Errorf("expected refuted result to become compliant")
	}
	if len(refuted.MissingRequirements) != 0 {
		t.Errorf("expected missing requirements cleared on refute")
	}
	if !strings.Contains(refuted.Summary, "Diff lines 40-60 implement") {
		t.Errorf("expected verifier rationale in summary, got: %s", refuted.Summary)
	}
}

func TestBusinessRulesValidationAgentProvider_Execute(t *testing.T) {
	ctx := context.Background()

	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role: contracts.RoleAssistant,
							Content: `{
								"is_compliant": true,
								"needs_more_info": false,
								"summary": "Verified all acceptance criteria in the diff.",
								"missing_requirements": []
							}`,
						},
					},
				},
			}, nil
		},
	}

	provider := NewBusinessRulesValidationAgentProvider(runner)

	bctx := BusinessRulesContext{
		TaskContext: "- [ ] Implement payment webhook handler",
		PRDiff:      "+ func HandleWebhook() { ... }",
	}

	result, err := provider.Execute(ctx, bctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsCompliant {
		t.Errorf("expected compliant, got %v", result)
	}
}

type mockTaskFetcher struct {
	fetchFn func(ctx context.Context, taskKeys []string) (string, error)
}

func (m *mockTaskFetcher) FetchTaskContext(ctx context.Context, taskKeys []string) (string, error) {
	return m.fetchFn(ctx, taskKeys)
}

func TestBusinessRulesValidationAgentProvider_DynamicTaskFetcher(t *testing.T) {
	ctx := context.Background()

	var promptPassed string
	runner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			promptPassed = input.Prompt
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: `{"is_compliant": true, "summary": "Ticket criteria met."}`,
						},
					},
				},
			}, nil
		},
	}

	fetcher := &mockTaskFetcher{
		fetchFn: func(ctx context.Context, taskKeys []string) (string, error) {
			for _, k := range taskKeys {
				if k == "PROJ-999" {
					return "Task: PROJ-999\nTitle: Add refund API\nAcceptance Criteria:\n- Must validate currency", nil
				}
			}
			return "", nil
		},
	}

	provider := NewBusinessRulesValidationAgentProvider(runner, WithTaskFetcher(fetcher))

	bctx := BusinessRulesContext{
		PRBody: "Implements PROJ-999 for refunds",
		PRDiff: "+ func ProcessRefund() {}",
	}

	result, err := provider.Execute(ctx, bctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsCompliant {
		t.Errorf("expected compliant, got %+v", result)
	}
	if !strings.Contains(promptPassed, "PROJ-999") || !strings.Contains(promptPassed, "Must validate currency") {
		t.Errorf("expected prompt to contain fetched task info, got: %s", promptPassed)
	}
}

func TestFormatBusinessRulesPRComment(t *testing.T) {
	if got := FormatBusinessRulesPRComment(nil, "PR #1"); got != "" {
		t.Errorf("expected empty string for nil result, got %q", got)
	}

	// 1. Needs more info
	infoRes := &ValidationResult{
		NeedsMoreInfo: true,
		MissingInfo:   "Please link a Jira task.",
	}
	infoComment := FormatBusinessRulesPRComment(infoRes, "feat: auth")
	if !strings.Contains(infoComment, "Needs More Information") || !strings.Contains(infoComment, "Please link a Jira task.") {
		t.Errorf("unexpected info comment: %s", infoComment)
	}

	// 2. Compliant
	compRes := &ValidationResult{
		IsCompliant: true,
		Confidence:  "high",
		Summary:     "Everything matches the spec.",
		Suggestions: []string{"Consider adding load tests"},
	}
	compComment := FormatBusinessRulesPRComment(compRes, "feat: payments")
	if !strings.Contains(compComment, "Compliant") || !strings.Contains(compComment, "Consider adding load tests") {
		t.Errorf("unexpected compliant comment: %s", compComment)
	}

	// 3. Issues found
	issueRes := &ValidationResult{
		IsCompliant:         false,
		Confidence:          "high",
		Summary:             "Missing critical requirements.",
		ViolatedRules:       []string{"Rule 1: Auth check required"},
		MissingRequirements: []string{"Audit log on failure"},
	}
	issueComment := FormatBusinessRulesPRComment(issueRes, "feat: sensitive")
	if !strings.Contains(issueComment, "Issues Found") ||
		!strings.Contains(issueComment, "Rule 1: Auth check required") ||
		!strings.Contains(issueComment, "Audit log on failure") {
		t.Errorf("unexpected issues comment: %s", issueComment)
	}
}

func TestBuildBusinessRulesContractViolationFeedback(t *testing.T) {
	inputFb := BuildBusinessRulesContractViolationFeedback("en", "input", []string{"organizationId", "pullRequestNumber"})
	if !strings.Contains(inputFb, "Missing Validation Context") || !strings.Contains(inputFb, "@drixy -v business-logic") {
		t.Errorf("unexpected input violation feedback: %s", inputFb)
	}
	if strings.Contains(inputFb, "@kody") || strings.Contains(inputFb, "kodus") {
		t.Errorf("detected brand leak in violation feedback: %s", inputFb)
	}

	outputFb := BuildBusinessRulesContractViolationFeedback("en", "output", []string{"is_compliant"})
	if !strings.Contains(outputFb, "Invalid Skill Response") || !strings.Contains(outputFb, "is_compliant") {
		t.Errorf("unexpected output violation feedback: %s", outputFb)
	}
}

func TestRequiredMcpFeedback(t *testing.T) {
	fb := BuildRequiredMcpFeedback(RequiredMcpFeedbackParams{
		RequiredMcps: []RequiredMcpInfo{
			{Category: "task-management", Label: "Jira", Examples: "Atlassian Jira Cloud"},
		},
		AvailableProviders: []string{"github"},
	})
	if !strings.Contains(fb, "MCP Integration Required") || !strings.Contains(fb, "Jira") || !strings.Contains(fb, "github") {
		t.Errorf("unexpected required mcp feedback: %s", fb)
	}

	failFb := BuildMcpConnectionFailureFeedback(McpConnectionFailureFeedbackParams{
		AvailableProviders: []string{"linear"},
	})
	if !strings.Contains(failFb, "MCP Connection Failed") || !strings.Contains(failFb, "linear") {
		t.Errorf("unexpected connection failure feedback: %s", failFb)
	}
}

type mockRuntimeToolCaller struct {
	tools map[string]func(ctx context.Context, args map[string]any) (*runtime.ToolExecutionResponse, error)
}

func (m *mockRuntimeToolCaller) CallTool(ctx context.Context, toolName string, args map[string]any) (*runtime.ToolExecutionResponse, error) {
	if fn, ok := m.tools[toolName]; ok {
		return fn(ctx, args)
	}
	return nil, fmt.Errorf("tool %s not found", toolName)
}

func (m *mockRuntimeToolCaller) CallAgent(ctx context.Context, agentName string, prompt string, options *runtime.AgentCallOptions) (*runtime.ToolExecutionResponse, error) {
	return nil, errors.New("unimplemented")
}

func (m *mockRuntimeToolCaller) GetRegisteredTools() []string {
	var res []string
	for k := range m.tools {
		res = append(res, k)
	}
	return res
}

func TestDefaultBusinessRulesBlueprintTooling(t *testing.T) {
	ctx := context.Background()

	caller := &mockRuntimeToolCaller{
		tools: map[string]func(ctx context.Context, args map[string]any) (*runtime.ToolExecutionResponse, error){
			"getPullRequest": func(ctx context.Context, args map[string]any) (*runtime.ToolExecutionResponse, error) {
				return &runtime.ToolExecutionResponse{
					Result: map[string]any{"body": "PR description from tool"},
				}, nil
			},
			"getPullRequestDiff": func(ctx context.Context, args map[string]any) (*runtime.ToolExecutionResponse, error) {
				return &runtime.ToolExecutionResponse{
					Result: map[string]any{"diff": "+ diff from tool"},
				}, nil
			},
			"getJiraIssue": func(ctx context.Context, args map[string]any) (*runtime.ToolExecutionResponse, error) {
				return &runtime.ToolExecutionResponse{
					Result: map[string]any{
						"key":         "PROJ-42",
						"summary":     "Implement auth",
						"description": "Must support SAML and OAuth",
					},
				}, nil
			},
		},
	}

	tooling := NewDefaultBusinessRulesBlueprintTooling(caller, nil, nil)

	// Fetch PR body
	body, err := tooling.FetchPullRequestBody(ctx, BusinessRulesContext{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	})
	if err != nil || body != "PR description from tool" {
		t.Fatalf("unexpected pr body: %v, err: %v", body, err)
	}

	// Fetch PR diff
	diff, err := tooling.FetchPullRequestDiff(ctx, BusinessRulesContext{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	})
	if err != nil || diff != "+ diff from tool" {
		t.Fatalf("unexpected pr diff: %v, err: %v", diff, err)
	}

	// Fetch Task context
	task, err := tooling.FetchTaskContext(ctx, BusinessRulesContext{
		OrganizationID: "org-1",
		TeamID:         "team-1",
	})
	if err != nil || task == nil || task.ID != "PROJ-42" {
		t.Fatalf("unexpected task context: %+v, err: %v", task, err)
	}
}
