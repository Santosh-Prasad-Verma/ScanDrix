// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
)

// AgentSpec is the declarative, pure-data configuration of an agent role.
// The harness runtime is shared; swapping SystemPrompt, Tools, and Policies
// instantiates different roles (finder, verifier, conversation, business rules)
// without forking the agent loop.
type AgentSpec struct {
	ID               string
	SystemPrompt     string
	AgentName        string
	RunName          string
	Phase            string
	SpanName         string
	Tools            ToolRegistry
	Policies         []AgentPolicy
	MaxSteps         int
	Temperature      *float64
	MaxOutputTokens  *int
	ProviderOptions  map[string]any
	ResultToolName   string
}

// AgentRunInput carries the invocation parameters for an agent execution.
type AgentRunInput struct {
	Prompt            string
	SeedMessages      []AgentMessage
	TelemetryMetadata map[string]any
	Telemetry         map[string]any
	RuntimeContext    map[string]any
}

// AgentRunner is the single agent execution loop engine.
// All roles (finder, verifier, sub-agents, replicas) execute through this port.
type AgentRunner interface {
	Run(ctx context.Context, spec AgentSpec, input AgentRunInput, toolCtx ToolContext) (*RunState, error)
}

// SubAgentParams defines configuration for wrapping an AgentSpec as an AgentTool.
type SubAgentParams struct {
	Name        string
	Description string
	Spec        AgentSpec
	InputSchema *JSONSchema
	ToPrompt    func(input any) string
	Summarize   func(state *RunState) string
}

// SubAgentFactory enables the sub-agent-as-tool pattern for hierarchical orchestration.
type SubAgentFactory interface {
	AsTool(params SubAgentParams) AgentTool
}
