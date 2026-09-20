// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Dynamic Skills Subsystem Tests
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package skills

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

type mockHarnessRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockHarnessRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

const sampleSkillFile = `---
name: sample-validation-skill
description: Tests PR code against business rules
allowed-tools: GET_PULL_REQUEST GET_PULL_REQUEST_DIFF
metadata:
    version: '1.2.0'
    scandrix:
        capabilities:
            - pr.metadata.read
            - pr.diff.read
            - task.context.read
        execution-policy:
            on-missing-mcp: fallback
            fetcher-timeout-ms: 60000
            analyzer-timeout-ms: 90000
            fetcher-max-iterations: 3
            analyzer-max-iterations: 1
            verify-analyzer-result: true
        contracts:
            input:
                required-context-fields:
                    - organization_id
                    - team_id
            output:
                required-fields:
                    - summary
                    - is_compliant
        required-mcps:
            - category: task-management
              label: Task Management
              examples: Jira, Linear
---

# Sample Skill System Prompt

You are an automated code validation agent. Compare PR diff against task context.
`

func TestParseSkillManifest(t *testing.T) {
	manifest, err := ParseSkillManifest(sampleSkillFile)
	if err != nil {
		t.Fatalf("unexpected error parsing manifest: %v", err)
	}

	if manifest.Name != "sample-validation-skill" {
		t.Errorf("expected name sample-validation-skill, got %s", manifest.Name)
	}
	if manifest.Version != "1.2.0" {
		t.Errorf("expected version 1.2.0, got %s", manifest.Version)
	}
	if len(manifest.AllowedTools) != 2 {
		t.Errorf("expected 2 allowed tools, got %v", manifest.AllowedTools)
	}
	if len(manifest.Capabilities) != 3 {
		t.Errorf("expected 3 capabilities, got %v", manifest.Capabilities)
	}
	if manifest.ExecutionPolicy.FetcherTimeoutMs != 60000 {
		t.Errorf("expected fetcher timeout 60000, got %d", manifest.ExecutionPolicy.FetcherTimeoutMs)
	}
	if !manifest.ExecutionPolicy.VerifyAnalyzerResult {
		t.Errorf("expected verify-analyzer-result=true")
	}
	if len(manifest.Contracts.RequiredContextFields) != 2 {
		t.Errorf("expected 2 required context fields, got %v", manifest.Contracts.RequiredContextFields)
	}
	if len(manifest.RequiredMcps) != 1 || manifest.RequiredMcps[0].Category != "task-management" {
		t.Errorf("expected 1 task-management required mcp, got %v", manifest.RequiredMcps)
	}
	if !strings.Contains(manifest.Instructions, "You are an automated code validation agent") {
		t.Errorf("expected instructions to contain system prompt body")
	}
}

func TestCapabilityRegistry(t *testing.T) {
	reg := NewCapabilityRegistry()
	caps := reg.List()
	if len(caps) < 3 {
		t.Errorf("expected at least 3 default capabilities, got %v", caps)
	}

	ctx := context.Background()

	// 1. Test PR Diff Read
	diffCap, ok := reg.Get("pr.diff.read")
	if !ok {
		t.Fatalf("missing pr.diff.read")
	}
	outDiff, err := diffCap.Execute(ctx, map[string]any{
		"pr_diff": "+ func Test() {}",
	})
	if err != nil || outDiff["pr_diff"] != "+ func Test() {}" {
		t.Errorf("unexpected diff output: %v, err: %v", outDiff, err)
	}

	// 2. Test Task Context Read
	taskCap, ok := reg.Get("task.context.read")
	if !ok {
		t.Fatalf("missing task.context.read")
	}
	outTask, err := taskCap.Execute(ctx, map[string]any{
		"task_context": "Requirements for billing",
	})
	if err != nil || outTask["task_context"] != "Requirements for billing" {
		t.Errorf("unexpected task output: %v, err: %v", outTask, err)
	}

	// 3. Test Task Context Read Dynamic Resolution
	outDynamic, err := taskCap.Execute(ctx, map[string]any{
		"pr_body": "Resolves JIRA-4321 for authorization",
		"task_fetcher": func(ctx context.Context, keys []string) (string, error) {
			if len(keys) > 0 && keys[0] == "JIRA-4321" {
				return "Dynamic requirement: must use OAuth2", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outDynamic["task_context"] != "Dynamic requirement: must use OAuth2" {
		t.Errorf("expected dynamic task context, got: %v", outDynamic["task_context"])
	}
	if outDynamic["has_task"] != true {
		t.Errorf("expected has_task=true")
	}
}

func TestGenericSkillRunner_Execute(t *testing.T) {
	manifest, err := ParseSkillManifest(sampleSkillFile)
	if err != nil {
		t.Fatalf("unexpected error parsing manifest: %v", err)
	}

	ctx := context.Background()
	runnerMock := &mockHarnessRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			if !strings.Contains(input.Prompt, "TASK_CONTEXT") {
				t.Errorf("expected prompt to contain TASK_CONTEXT")
			}
			if !strings.Contains(input.Prompt, "PR_DIFF") {
				t.Errorf("expected prompt to contain PR_DIFF")
			}
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Steps: []contracts.RunStep{
					{
						Index: 0,
						Message: contracts.AgentMessage{
							Role:    contracts.RoleAssistant,
							Content: `{"is_compliant": true, "summary": "All checks passed."}`,
						},
					},
				},
			}, nil
		},
	}

	skillRunner := NewGenericSkillRunner(runnerMock, NewCapabilityRegistry())

	res, err := skillRunner.Execute(ctx, SkillRunInput{
		Manifest:       manifest,
		OrganizationID: "org-1",
		TeamID:         "team-1",
		Context: map[string]any{
			"task_context": "Task: Implement feature A",
			"pr_diff":      "+ feature A code",
		},
	})

	if err != nil {
		t.Fatalf("unexpected skill execution error: %v", err)
	}
	if res.SkillName != "sample-validation-skill" {
		t.Errorf("expected skill name sample-validation-skill, got %s", res.SkillName)
	}
	if res.ParsedOutput == nil || res.ParsedOutput["is_compliant"] != true {
		t.Errorf("expected compliant parsed output, got %v", res.ParsedOutput)
	}
	if !res.Verified {
		t.Errorf("expected verified=true from manifest")
	}
}

func TestGenericSkillRunner_ContractEnforcement(t *testing.T) {
	manifest, err := ParseSkillManifest(sampleSkillFile)
	if err != nil {
		t.Fatalf("unexpected error parsing manifest: %v", err)
	}

	ctx := context.Background()
	skillRunner := NewGenericSkillRunner(&mockHarnessRunner{}, NewCapabilityRegistry())

	// Missing organization_id
	_, err = skillRunner.Execute(ctx, SkillRunInput{
		Manifest:       manifest,
		OrganizationID: "",
		TeamID:         "team-1",
	})
	if err == nil || !strings.Contains(err.Error(), "contract violation") {
		t.Fatalf("expected contract violation error, got: %v", err)
	}
}
