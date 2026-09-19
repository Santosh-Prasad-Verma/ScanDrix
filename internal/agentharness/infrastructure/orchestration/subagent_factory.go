// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orchestration

import (
	"fmt"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// DefaultInputSchema is the default JSON Schema when a sub-agent does not specify one.
var DefaultInputSchema = contracts.JSONSchema{
	Type: "object",
	Properties: map[string]contracts.JSONSchema{
		"task": {
			Type:        "string",
			Description: "The task for the sub-agent.",
		},
	},
	Required: []string{"task"},
}

// subAgentToolImpl wraps an AgentSpec as an executable AgentTool.
type subAgentToolImpl struct {
	name        string
	description string
	spec        contracts.AgentSpec
	inputSchema contracts.JSONSchema
	toPrompt    func(input any) string
	summarize   func(state *contracts.RunState) string
	runner      contracts.AgentRunner
}

func (s *subAgentToolImpl) Name() string {
	return s.name
}

func (s *subAgentToolImpl) Description() string {
	return s.description
}

func (s *subAgentToolImpl) InputSchema() contracts.JSONSchema {
	return s.inputSchema
}

func (s *subAgentToolImpl) Strict() bool {
	return false
}

func (s *subAgentToolImpl) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	prompt := ""
	if s.toPrompt != nil {
		prompt = s.toPrompt(input)
	} else if m, ok := input.(map[string]any); ok {
		if task, ok := m["task"].(string); ok {
			prompt = task
		}
	} else {
		prompt = fmt.Sprintf("%v", input)
	}

	state, err := s.runner.Run(ctx.Context, s.spec, contracts.AgentRunInput{
		Prompt: prompt,
	}, ctx)
	if err != nil {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("sub-agent run failed: %v", err),
			IsError: true,
			Meta: map[string]any{
				"subAgentId": s.spec.ID,
				"error":      err.Error(),
			},
		}, nil
	}

	summary := ""
	if s.summarize != nil {
		summary = s.summarize(state)
	} else {
		// Default to FinalText
		for i := len(state.Steps) - 1; i >= 0; i-- {
			if str, ok := state.Steps[i].Message.Content.(string); ok && str != "" {
				summary = str
				break
			}
		}
	}

	return contracts.ToolResult{
		Output: summary,
		Meta: map[string]any{
			"subAgentId": s.spec.ID,
			"status":     string(state.Status),
			"steps":      len(state.Steps),
		},
	}, nil
}

// DefaultSubAgentFactory implements the SubAgentFactory contract.
type DefaultSubAgentFactory struct {
	runner contracts.AgentRunner
}

// NewDefaultSubAgentFactory constructs a DefaultSubAgentFactory.
func NewDefaultSubAgentFactory(runner contracts.AgentRunner) *DefaultSubAgentFactory {
	return &DefaultSubAgentFactory{runner: runner}
}

// AsTool turns an AgentSpec into an executable AgentTool that a parent agent can invoke.
func (f *DefaultSubAgentFactory) AsTool(params contracts.SubAgentParams) contracts.AgentTool {
	schema := DefaultInputSchema
	if params.InputSchema != nil {
		schema = *params.InputSchema
	}

	return &subAgentToolImpl{
		name:        params.Name,
		description: params.Description,
		spec:        params.Spec,
		inputSchema: schema,
		toPrompt:    params.ToPrompt,
		summarize:   params.Summarize,
		runner:      f.runner,
	}
}
