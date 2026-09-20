// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Dynamic Skills Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

// SkillRunInput holds the full parameters required to execute an agent skill.
type SkillRunInput struct {
	Manifest       *SkillManifest         `json:"-"`
	OrganizationID string                 `json:"organization_id"`
	TeamID         string                 `json:"team_id"`
	RepositoryID   string                 `json:"repository_id,omitempty"`
	ThreadID       string                 `json:"thread_id,omitempty"`
	Context        map[string]any         `json:"context"`
	Tools          contracts.ToolRegistry `json:"-"`
	CustomPrompt   string                 `json:"custom_prompt,omitempty"`
}

// SkillRunResult encapsulates the output, execution traces, and timing of a skill run.
type SkillRunResult struct {
	SkillName    string         `json:"skill_name"`
	RawOutput    string         `json:"raw_output"`
	ParsedOutput map[string]any `json:"parsed_output,omitempty"`
	StepCount    int            `json:"step_count"`
	Duration     time.Duration  `json:"duration"`
	Verified     bool           `json:"verified"`
}

// GenericSkillRunner coordinates the multi-stage execution of any loaded skill.
type GenericSkillRunner struct {
	runner       contracts.AgentRunner
	capabilities *CapabilityRegistry
}

// NewGenericSkillRunner constructs a runner with the agent harness runner and capability registry.
func NewGenericSkillRunner(
	runner contracts.AgentRunner,
	capabilities *CapabilityRegistry,
) *GenericSkillRunner {
	if capabilities == nil {
		capabilities = NewCapabilityRegistry()
	}
	return &GenericSkillRunner{
		runner:       runner,
		capabilities: capabilities,
	}
}

// Execute runs the skill through pre-flight validation, capability resolution, and analyzer orchestration.
func (r *GenericSkillRunner) Execute(ctx context.Context, input SkillRunInput) (*SkillRunResult, error) {
	start := time.Now()

	if input.Manifest == nil {
		return nil, errors.New("skill manifest cannot be nil")
	}
	if r.runner == nil {
		return nil, errors.New("agent harness runner unavailable")
	}

	manifest := input.Manifest

	// 1. Contract Pre-Flight Validation
	if err := r.validateInputContracts(manifest.Contracts.RequiredContextFields, input); err != nil {
		return nil, fmt.Errorf("contract violation: %w", err)
	}

	// 2. Resolve Abstract Capabilities
	resolvedContext := make(map[string]any)
	for k, v := range input.Context {
		resolvedContext[k] = v
	}

	for _, capName := range manifest.Capabilities {
		if cap, ok := r.capabilities.Get(capName); ok {
			capOutput, err := cap.Execute(ctx, resolvedContext)
			if err == nil && capOutput != nil {
				for k, v := range capOutput {
					resolvedContext[k] = v
				}
			}
		}
	}

	// 3. Build Analyzer Prompt & Context Envelope
	promptBuilder := strings.Builder{}
	promptBuilder.WriteString(fmt.Sprintf("## Skill: %s\n\n", manifest.Name))

	if taskCtx, ok := resolvedContext["task_context"].(string); ok && strings.TrimSpace(taskCtx) != "" {
		promptBuilder.WriteString("### TASK_CONTEXT\n")
		promptBuilder.WriteString(taskCtx)
		promptBuilder.WriteString("\n\n")
	}

	if prDiff, ok := resolvedContext["pr_diff"].(string); ok && strings.TrimSpace(prDiff) != "" {
		promptBuilder.WriteString("### PR_DIFF\n")
		promptBuilder.WriteString(prDiff)
		promptBuilder.WriteString("\n\n")
	}

	if input.CustomPrompt != "" {
		promptBuilder.WriteString("### USER_INSTRUCTION\n")
		promptBuilder.WriteString(input.CustomPrompt)
		promptBuilder.WriteString("\n\n")
	}

	toolReg := input.Tools
	if toolReg == nil {
		toolReg = tools.NewInMemoryToolRegistry()
	}

	maxTokens := 20000
	maxSteps := manifest.ExecutionPolicy.AnalyzerMaxIterations
	if maxSteps <= 0 {
		maxSteps = 2
	}

	spec := contracts.AgentSpec{
		ID:              manifest.Name,
		AgentName:       manifest.Name,
		RunName:         "GenericSkillRunner::" + manifest.Name,
		Phase:           "skillExecution",
		SpanName:        "Skill::" + manifest.Name,
		SystemPrompt:    manifest.Instructions,
		Tools:           toolReg,
		MaxSteps:        maxSteps,
		MaxOutputTokens: &maxTokens,
	}

	toolCtx := contracts.ToolContext{
		RunID:   fmt.Sprintf("skill-%s-%d", manifest.Name, time.Now().UnixNano()),
		Context: ctx,
		Services: map[string]any{
			"organization_id": input.OrganizationID,
			"team_id":         input.TeamID,
			"repository_id":   input.RepositoryID,
		},
	}

	runInput := contracts.AgentRunInput{
		Prompt: promptBuilder.String(),
		RuntimeContext: map[string]any{
			"thread_id": input.ThreadID,
		},
	}

	// 4. Execution via Agent Harness Runner
	state, err := r.runner.Run(ctx, spec, runInput, toolCtx)
	if err != nil {
		return nil, fmt.Errorf("skill execution failed: %w", err)
	}

	rawOutput := ""
	if state != nil && len(state.Steps) > 0 {
		lastIdx := len(state.Steps) - 1
		switch c := state.Steps[lastIdx].Message.Content.(type) {
		case string:
			rawOutput = c
		default:
			rawOutput = fmt.Sprintf("%v", c)
		}
	}

	// 5. Parse Output if JSON
	var parsed map[string]any
	trimmed := strings.TrimSpace(rawOutput)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		_ = json.Unmarshal([]byte(trimmed), &parsed)
	}

	return &SkillRunResult{
		SkillName:    manifest.Name,
		RawOutput:    rawOutput,
		ParsedOutput: parsed,
		StepCount:    len(state.Steps),
		Duration:     time.Since(start),
		Verified:     manifest.ExecutionPolicy.VerifyAnalyzerResult,
	}, nil
}

func (r *GenericSkillRunner) validateInputContracts(requiredFields []string, input SkillRunInput) error {
	for _, field := range requiredFields {
		switch field {
		case "organizationAndTeamData.organizationId", "organization_id":
			if input.OrganizationID == "" {
				return fmt.Errorf("missing required field: %s", field)
			}
		case "organizationAndTeamData.teamId", "team_id":
			if input.TeamID == "" {
				return fmt.Errorf("missing required field: %s", field)
			}
		case "prepareContext.repository.id", "repository_id":
			if input.RepositoryID == "" {
				if repoID, ok := input.Context["repository_id"].(string); !ok || repoID == "" {
					return fmt.Errorf("missing required field: %s", field)
				}
			}
		default:
			// Check general context map
			if _, ok := input.Context[field]; !ok {
				parts := strings.Split(field, ".")
				if len(parts) > 1 {
					// Check nested map
					if sub, ok := input.Context[parts[0]].(map[string]any); ok {
						if _, subOk := sub[parts[1]]; subOk {
							continue
						}
					}
				}
				return fmt.Errorf("missing required context field: %s", field)
			}
		}
	}
	return nil
}
