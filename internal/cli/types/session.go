package types

import (
	"time"
)

// SessionTurnType defines the type of event in a session turn.
type SessionTurnType string

const (
	TurnTypeUserInput       SessionTurnType = "user_input"
	TurnTypeAgentResponse   SessionTurnType = "agent_response"
	TurnTypeToolCall        SessionTurnType = "tool_call"
	TurnTypeToolResult      SessionTurnType = "tool_result"
	TurnTypeError           SessionTurnType = "error"
	TurnTypeReviewRequested SessionTurnType = "review_requested"
	TurnTypeReviewCompleted SessionTurnType = "review_completed"
)

// ToolInvocation represents a single tool execution during an agent session.
type ToolInvocation struct {
	ToolName   string         `json:"tool_name"`
	Input      map[string]any `json:"input,omitempty"`
	Output     string         `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"duration_ms"`
	Timestamp  time.Time      `json:"timestamp"`
}

// SessionTurn represents a single conversational turn in an agent or CLI session.
type SessionTurn struct {
	TurnIndex  int              `json:"turn_index"`
	Type       SessionTurnType  `json:"type"`
	Sender     string           `json:"sender"` // user, assistant, system, tool
	Content    string           `json:"content"`
	ToolCalls  []ToolInvocation `json:"tool_calls,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
	DurationMs int64            `json:"duration_ms"`
	TokensUsed int              `json:"tokens_used,omitempty"`
}

// SessionMetadata holds context around an interactive coding session.
type SessionMetadata struct {
	SessionID     string       `json:"session_id"`
	AgentType     string       `json:"agent_type"` // claude_code, cursor, codex, terminal
	Branch        string       `json:"branch"`
	HeadSHA       string       `json:"head_sha"`
	RepositoryURL string       `json:"repository_url"`
	StartTime     time.Time    `json:"start_time"`
	EndTime       *time.Time   `json:"end_time,omitempty"`
	TurnCount     int          `json:"turn_count"`
	TotalTokens   int          `json:"total_tokens"`
	Tags          []string     `json:"tags,omitempty"`
	Attributes    map[string]any `json:"attributes,omitempty"`
}

// Session represents a complete recorded coding and review session.
type Session struct {
	Metadata SessionMetadata `json:"metadata"`
	Turns    []SessionTurn   `json:"turns"`
}
