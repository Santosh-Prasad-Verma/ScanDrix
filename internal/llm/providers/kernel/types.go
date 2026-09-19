// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package kernel

import (
	"context"

	"github.com/scandrix/backend/internal/llm/byok"
)

// ReasoningEffort represents canonical reasoning intensity.
type ReasoningEffort string

const (
	ReasoningNone   ReasoningEffort = "none"
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
)

// ModelCapabilities encapsulates execution and feature capabilities of a model.
type ModelCapabilities struct {
	MaxInputTokens      int    `json:"max_input_tokens,omitempty"`
	StructuredOutput    string `json:"structured_output,omitempty"` // "json_schema", "json_object", "none"
	ToolCalling         string `json:"tool_calling,omitempty"`      // "native", "none"
	SupportsStreaming   bool   `json:"supports_streaming,omitempty"`
	PromptCaching       bool   `json:"prompt_caching,omitempty"`
	SupportsReasoning   bool   `json:"supports_reasoning,omitempty"`
	SupportsTemperature bool   `json:"supports_temperature,omitempty"`
}

// ModelReasoningTraits declares model-specific reasoning facts.
type ModelReasoningTraits struct {
	ThinksByDefault                bool   `json:"thinks_by_default"`
	CanDisableThinking             bool   `json:"can_disable_thinking"`
	ForcedToolChoiceSupported      bool   `json:"forced_tool_choice_supported"`
	RejectsThinkingWhenToolsForced bool   `json:"rejects_thinking_when_tools_forced"`
	BudgetMode                     string `json:"budget_mode,omitempty"` // "fixed", "range", "effort_only", "none"
}

// Temperature policy constants.
const (
	TemperatureFree        = "free"
	TemperatureFixed       = "fixed"
	TemperatureUnsupported = "unsupported"
)

// TemperaturePolicy declares how a model treats temperature.
type TemperaturePolicy struct {
	Mode       string   `json:"mode"` // "free", "fixed", "unsupported"
	Kind       string   `json:"kind,omitempty"`
	FixedValue *float64 `json:"fixed_value,omitempty"`
	Value      *float64 `json:"value,omitempty"`
}

// ChatMessage represents a single message in an LLM conversation.
type ChatMessage struct {
	Role       string     `json:"role"` // "system", "user", "assistant", "tool"
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolDefinition defines a callable function or tool.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// TokenUsage captures token consumption details.
type TokenUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
	TotalTokens     int `json:"total_tokens"`
}

// ExecutionRequest encapsulates execution parameters for a provider call.
type ExecutionRequest struct {
	Messages        []ChatMessage
	Tools           []ToolDefinition
	ToolChoice      any // "auto", "required", "none", or specific tool
	Temperature     *float64
	MaxTokens       int
	ReasoningEffort ReasoningEffort
	ResponseSchema  map[string]any
	SystemCacheHint bool
}

// ExecutionResult contains output from an executed provider call.
type ExecutionResult struct {
	Text      string
	ToolCalls []ToolCall
	Usage     TokenUsage
	Raw       any
}

// CatalogModel represents an individual model entry in a provider's model catalog.
type CatalogModel struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	SupportsReasoning bool           `json:"supports_reasoning,omitempty"`
	ReasoningConfig   map[string]any `json:"reasoning_config,omitempty"`
}

// ResolvedListingCreds holds credentials resolved by the caller before a model listing HTTP request.
type ResolvedListingCreds struct {
	APIKey         string
	BaseURL        string
	AWSBearerToken string
	AWSRegion      string
}

// ModelListingKind defines the model enumeration mechanism.
type ModelListingKind string

const (
	ListingManual ModelListingKind = "manual"
	ListingStatic ModelListingKind = "static"
	ListingHTTP   ModelListingKind = "http"
)

// ModelListing describes how to enumerate a provider's models.
type ModelListing struct {
	Kind            ModelListingKind
	APIKeyEnv       string
	BaseURLEnv      string
	DefaultBaseURL  string
	RequiresBaseURL bool
	TimeoutMs       int
	StaticModels    []CatalogModel
	FallbackModels  []CatalogModel
	URL             func(creds ResolvedListingCreds) string
	Headers         func(creds ResolvedListingCreds) map[string]string
	Parse           func(body []byte) ([]CatalogModel, error)
}

// FieldDescriptor describes a UI configuration field in BYOK settings.
type FieldDescriptor struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Type        string     `json:"type"` // "text" | "password" | "url" | "select" | "number" | "boolean"
	Required    bool       `json:"required,omitempty"`
	Placeholder string     `json:"placeholder,omitempty"`
	Options     []FieldOpt `json:"options,omitempty"`
	Scope       string     `json:"scope,omitempty"` // "top" | "settings"
}

// FieldOpt defines an option choice for select fields.
type FieldOpt struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ProviderModule defines the self-describing contract that each AI provider implements.
type ProviderModule interface {
	ID() string
	Aliases() []string
	Label() string
	Doc() string
	Capabilities(model string) ModelCapabilities
	ReasoningTraits(cfg byok.NormalizedModel) ModelReasoningTraits
	TemperaturePolicy(cfg byok.NormalizedModel) *TemperaturePolicy
	SystemCacheControl(cfg byok.NormalizedModel) map[string]any
	Execute(ctx context.Context, cfg byok.NormalizedModel, req ExecutionRequest) (*ExecutionResult, error)
	UIFields() []FieldDescriptor
	ModelListing(providerID string) *ModelListing
}
