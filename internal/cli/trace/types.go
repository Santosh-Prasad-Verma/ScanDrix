// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"time"
)

// AgentType defines supported AI agent engines.
type AgentType string

const (
	AgentClaudeCode AgentType = "claude-code"
	AgentCursor     AgentType = "cursor"
	AgentCodex      AgentType = "codex"
)

// TokenUsage tracks prompt, completion, and cache tokens.
type TokenUsage struct {
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	TotalTokens      int64 `json:"total_tokens,omitempty"`
}

// FileChange describes modifications to a file path.
type FileChange struct {
	Path      string `json:"path"`
	Additions int    `json:"additions,omitempty"`
	Deletions int    `json:"deletions,omitempty"`
}

// TraceToolCallRecord captures an individual tool invocation by an agent.
type TraceToolCallRecord struct {
	ToolName     string `json:"tool_name"`
	Summary      string `json:"summary,omitempty"`
	FileAffected string `json:"file_affected,omitempty"`
}

// TraceRecordLine is the unified polymorphic line structure in ~/.scandrix/sessions/<repoKey>/records/<id>.jsonl
type TraceRecordLine struct {
	Kind          string                `json:"kind"` // session-start, turn-start, turn-end, session-end
	SessionID     string                `json:"session_id,omitempty"`
	AgentType     AgentType             `json:"agent_type,omitempty"`
	Branch        string                `json:"branch,omitempty"`
	BaseCommit    string                `json:"base_commit,omitempty"`
	GitRemote     string                `json:"git_remote,omitempty"`
	CLIVersion    string                `json:"cli_version,omitempty"`
	TurnID        string                `json:"turn_id,omitempty"`
	Prompt        string                `json:"prompt,omitempty"` // Scrubbed
	Response      string                `json:"response,omitempty"` // Scrubbed
	CommitBefore  string                `json:"commit_before,omitempty"`
	CommitAfter   string                `json:"commit_after,omitempty"`
	ToolCalls     []TraceToolCallRecord `json:"tool_calls,omitempty"`
	FilesModified []FileChange          `json:"files_modified,omitempty"`
	FilesRead     []string              `json:"files_read,omitempty"`
	Commands      []string              `json:"commands,omitempty"`
	TokenUsage    *TokenUsage           `json:"token_usage,omitempty"`
	Timestamp     string                `json:"timestamp"`
}

// TraceTurn is a hydrated user/assistant turn.
type TraceTurn struct {
	TurnID        string                `json:"turn_id"`
	Prompt        string                `json:"prompt"`
	Response      string                `json:"response"`
	ToolCalls     []TraceToolCallRecord `json:"tool_calls"`
	FilesModified []FileChange          `json:"files_modified"`
	FilesRead     []string              `json:"files_read"`
	Commands      []string              `json:"commands"`
	TokenUsage    *TokenUsage           `json:"token_usage,omitempty"`
	CommitBefore  string                `json:"commit_before,omitempty"`
	CommitAfter   string                `json:"commit_after,omitempty"`
	StartedAt     string                `json:"started_at,omitempty"`
	EndedAt       string                `json:"ended_at,omitempty"`
}

// TraceSession aggregates an entire agent session.
type TraceSession struct {
	SessionID    string      `json:"session_id"`
	AgentType    AgentType   `json:"agent_type,omitempty"`
	Branch       string      `json:"branch,omitempty"`
	BaseCommit   string      `json:"base_commit,omitempty"`
	GitRemote    string      `json:"git_remote,omitempty"`
	CLIVersion   string      `json:"cli_version,omitempty"`
	StartedAt    string      `json:"started_at,omitempty"`
	EndedAt      string      `json:"ended_at,omitempty"`
	Turns        []TraceTurn `json:"turns"`
	CorruptLines int         `json:"corrupt_lines,omitempty"`
}

// DECISION TYPES & GIT ORPHAN BRANCH RECORDS

type DecisionType string

const (
	DecisionTypeArchitectural DecisionType = "architectural_decision"
	DecisionTypeConvention    DecisionType = "convention"
	DecisionTypeTradeoff      DecisionType = "tradeoff"
	DecisionTypeDetail        DecisionType = "implementation_detail"
	DecisionTypeTooling       DecisionType = "tooling"
	DecisionTypeOther         DecisionType = "other"
)

// Decision models a durable architectural decision distilled from agent sessions.
type Decision struct {
	ID         string       `json:"id"` // Stable SHA-256 hash(branch, decision, scope)
	Type       DecisionType `json:"type"`
	Origin     string       `json:"origin,omitempty"` // human, agent, collaborative
	Decision   string       `json:"decision"`
	Rationale  string       `json:"rationale,omitempty"`
	Confidence float64      `json:"confidence,omitempty"`
	Evidence   []string     `json:"evidence,omitempty"`
	Scope      []string     `json:"scope"` // File paths / dir prefixes
	Pinned     bool         `json:"pinned,omitempty"`
	Branch     string       `json:"branch,omitempty"`
	Commits    []string     `json:"commits,omitempty"`
	SessionIDs []string     `json:"session_ids,omitempty"`
	CreatedAt  string       `json:"created_at,omitempty"`
}

// Overrides tracks user pin/forget modifications to distilled decisions.
type Overrides struct {
	Pins    []string `json:"pins,omitempty"`    // Decision IDs pinned
	Forgets []string `json:"forgets,omitempty"` // Decision IDs forgotten/tombstoned
}

// TraceBranchRecord is the atomic JSON record stored on the orphan git branch (scandrix/trace/v1).
type TraceBranchRecord struct {
	Version     int        `json:"version"` // 1
	Branch      string     `json:"branch"`
	MergeBase   string     `json:"merge_base"`
	Head        string     `json:"head"`
	Commits     []string   `json:"commits"`
	UpdatedAt   string     `json:"updated_at"`
	Decisions   []Decision `json:"decisions"`
	Corrections *Overrides `json:"corrections,omitempty"`
}

// TraceIncident captures an operational trace error reported by status.
type TraceIncident struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Context string `json:"context,omitempty"`
}

// Type aliases for backwards compatibility and ergonomic usage
type Incident = TraceIncident
type TraceDecision = Decision
type TraceSessionSummary = SessionSummary
type TraceOverrides = Overrides

// LifecycleEvent defines the normalized payload passed from agent adapters to the lifecycle coordinator.
type LifecycleEvent struct {
	Type            string    `json:"type"` // SessionStart, TurnStart, TurnEnd, SessionEnd, SubagentStart, SubagentEnd
	SessionID       string    `json:"session_id"`
	SessionRef      string    `json:"session_ref,omitempty"` // Path to transcript
	Prompt          string    `json:"prompt,omitempty"`
	ToolUseID       string    `json:"tool_use_id,omitempty"`
	SubagentID      string    `json:"subagent_id,omitempty"`
	SubagentType    string    `json:"subagent_type,omitempty"`
	TaskDescription string    `json:"task_description,omitempty"`
	ToolInput       any       `json:"tool_input,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}
