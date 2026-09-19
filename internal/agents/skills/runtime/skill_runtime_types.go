package runtime

import (
	"context"
	"time"
)

// AgentThread represents a correlation thread across the skill runtime.
type AgentThread struct {
	ID       string         `json:"id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolExecutionResponse represents the response from an executed tool.
type ToolExecutionResponse struct {
	Result any `json:"result,omitempty"`
}

// AgentCallOptions parameters for calling delegated agents.
type AgentCallOptions struct {
	Thread      *AgentThread   `json:"thread,omitempty"`
	UserContext map[string]any `json:"userContext,omitempty"`
}

// ToolCaller defines the interface for calling tools and sub-agents.
type ToolCaller interface {
	CallTool(ctx context.Context, toolName string, args map[string]any) (*ToolExecutionResponse, error)
	CallAgent(ctx context.Context, agentName string, prompt string, options *AgentCallOptions) (*ToolExecutionResponse, error)
	GetRegisteredTools() []string
}

// SkillCapabilityDefinition describes an individual capability.
type SkillCapabilityDefinition struct {
	Description string   `json:"description"`
	Tools       []string `json:"tools,omitempty"`
	Requires    []string `json:"requires,omitempty"`
}

// SkillFetcherPolicy defines caching and timeout policies.
type SkillFetcherPolicy struct {
	CacheTTLMs int `json:"cacheTtlMs"`
	TimeoutMs  int `json:"timeoutMs"`
}

// SkillCapabilityRuntimeConfig holds runtime parameters for capabilities.
type SkillCapabilityRuntimeConfig struct {
	Capabilities          []string                             `json:"capabilities"`
	AllowedTools          []string                             `json:"allowedTools"`
	CapabilityToolMap     map[string][]string                  `json:"capabilityToolMap,omitempty"`
	CapabilityDefinitions map[string]SkillCapabilityDefinition `json:"capabilityDefinitions,omitempty"`
	FetcherPolicy         SkillFetcherPolicy                   `json:"fetcherPolicy"`
	ProviderType          string                               `json:"providerType"`
	AllProviderTypes      []string                             `json:"allProviderTypes,omitempty"`
}

// CapabilityExecutionMode indicates whether tool was run deterministically or via agent loop.
type CapabilityExecutionMode string

const (
	ModeDeterministic CapabilityExecutionMode = "deterministic"
	ModeAgentic       CapabilityExecutionMode = "agentic"
)

// CapabilityExecutionStatus indicates execution outcome.
type CapabilityExecutionStatus string

const (
	StatusSuccess CapabilityExecutionStatus = "success"
	StatusFailed  CapabilityExecutionStatus = "failed"
	StatusSkipped CapabilityExecutionStatus = "skipped"
)

// CapabilityExecutionTrace records telemetry for a capability run.
type CapabilityExecutionTrace struct {
	OrganizationID string                    `json:"organizationId"`
	TeamID         string                    `json:"teamId"`
	SkillName      string                    `json:"skillName"`
	Capability     string                    `json:"capability"`
	Provider       string                    `json:"provider"`
	Mode           CapabilityExecutionMode   `json:"mode"`
	Status         CapabilityExecutionStatus `json:"status"`
	ToolName       string                    `json:"toolName,omitempty"`
	Reason         string                    `json:"reason,omitempty"`
	LatencyMs      int64                     `json:"latencyMs"`
	OccurredAt     time.Time                 `json:"occurredAt"`
}

// CapabilityStrategyScope scopes strategy decisions per org/team/skill/provider.
type CapabilityStrategyScope struct {
	OrganizationID string `json:"organizationId"`
	TeamID         string `json:"teamId"`
	SkillName      string `json:"skillName"`
	Capability     string `json:"capability"`
	Provider       string `json:"provider"`
}

// CapabilityExecutionHooks allows injecting strategy hooks.
type CapabilityExecutionHooks struct {
	ResolvePreferredTool       func(ctx context.Context, scope CapabilityStrategyScope, candidateTools []string) (string, bool)
	GetCachedTaskContextTools  func(ctx context.Context, scope CapabilityStrategyScope) []string
	SaveCachedTaskContextTools func(ctx context.Context, scope CapabilityStrategyScope, tools []string)
	GetSeedTaskContextTools    func(ctx context.Context, providerType, capability string) []string
	ResolveTaskContextMode     func(ctx context.Context, providerType string) string
	RecordExecution            func(ctx context.Context, trace CapabilityExecutionTrace)
}
