// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import "time"

// AgentRole defines the role of a message participant in the agent loop.
type AgentRole string

const (
	RoleSystem    AgentRole = "system"
	RoleUser      AgentRole = "user"
	RoleAssistant AgentRole = "assistant"
	RoleTool      AgentRole = "tool"
)

// ToolCallRecord captures the identity, input, execution result, and duration
// of an individual tool call within a step.
type ToolCallRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Input      any    `json:"input"`
	Output     string `json:"output,omitempty"`
	IsError    bool   `json:"is_error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// AgentMessage represents an individual turn in the conversation history.
// Content can be plain text string or structured content parts.
type AgentMessage struct {
	Role       AgentRole        `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []ToolCallRecord `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

// TokenUsage records granular token counts reported by providers or estimated.
type TokenUsage struct {
	InputTokens      int `json:"input_tokens,omitempty"`
	OutputTokens     int `json:"output_tokens,omitempty"`
	ReasoningTokens  int `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// Total returns the total token count of input + output + reasoning.
func (u TokenUsage) Total() int {
	return u.InputTokens + u.OutputTokens + u.ReasoningTokens
}

// Add sums two TokenUsage values.
func (u TokenUsage) Add(other TokenUsage) TokenUsage {
	return TokenUsage{
		InputTokens:      u.InputTokens + other.InputTokens,
		OutputTokens:     u.OutputTokens + other.OutputTokens,
		ReasoningTokens:  u.ReasoningTokens + other.ReasoningTokens,
		CacheReadTokens:  u.CacheReadTokens + other.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens + other.CacheWriteTokens,
	}
}

// RunStep represents a single completed turn of the agent loop.
type RunStep struct {
	Index   int          `json:"index"`
	Message AgentMessage `json:"message"`
	Usage   *TokenUsage  `json:"usage,omitempty"`
}

// Artifact represents a raw structured output emitted by the agent
// (e.g. captured via resultToolName).
type Artifact struct {
	Type     string `json:"type"`
	Payload  any    `json:"payload"`
	Location string `json:"location,omitempty"`
	Stage    string `json:"stage,omitempty"`
}

// TraceEvent represents an append-only observability entry stamped during execution.
type TraceEvent struct {
	At     time.Time      `json:"at"`
	Source string         `json:"source"`
	Kind   string         `json:"kind"`
	Detail map[string]any `json:"detail,omitempty"`
}

// RunStatus represents the final disposition of an agent run.
type RunStatus string

const (
	StatusCompleted       RunStatus = "completed"
	StatusStopped         RunStatus = "stopped"
	StatusBudgetExhausted RunStatus = "budget-exhausted"
	StatusError           RunStatus = "error"
)

// RunState is the first-class, observable record of an agent run.
// Every step, tool call, artifact, and trace event is recorded by construction.
type RunState struct {
	RunID      string       `json:"run_id"`
	AgentID    string       `json:"agent_id"`
	Status     RunStatus    `json:"status"`
	Steps      []RunStep    `json:"steps"`
	Artifacts  []Artifact   `json:"artifacts"`
	StopReason string       `json:"stop_reason,omitempty"`
	Usage      TokenUsage   `json:"usage"`
	Trace      []TraceEvent `json:"trace"`
}
