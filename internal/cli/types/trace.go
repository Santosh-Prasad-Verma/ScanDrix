package types

import (
	"time"
)

// DecisionStatus represents outcome of a decision node.
type DecisionStatus string

const (
	DecisionStatusProposed DecisionStatus = "proposed"
	DecisionStatusAccepted DecisionStatus = "accepted"
	DecisionStatusRejected DecisionStatus = "rejected"
	DecisionStatusReverted DecisionStatus = "reverted"
)

// DecisionRecord represents an architectural or code decision captured in session trace.
type DecisionRecord struct {
	ID          string         `json:"id"`
	TurnIndex   int            `json:"turn_index"`
	Prompt      string         `json:"prompt"`
	Decision    string         `json:"decision"`
	Rationale   string         `json:"rationale"`
	Alternatives []string      `json:"alternatives,omitempty"`
	Status      DecisionStatus `json:"status"`
	FilesTouched []string      `json:"files_touched,omitempty"`
	Timestamp   time.Time      `json:"timestamp"`
}

// DecisionBranch represents a branch in the exploration graph of an agent session.
type DecisionBranch struct {
	BranchID   string           `json:"branch_id"`
	ParentID   string           `json:"parent_id,omitempty"`
	Label      string           `json:"label"`
	Decisions  []DecisionRecord `json:"decisions"`
	IsDeadEnd  bool             `json:"is_dead_end"`
	DeadEndReason string        `json:"dead_end_reason,omitempty"`
	CreatedAt  time.Time        `json:"created_at"`
}

// DistillSummary holds distilled insights extracted from a session trace.
type DistillSummary struct {
	SessionID       string           `json:"session_id"`
	Branch          string           `json:"branch"`
	HeadSHA         string           `json:"head_sha"`
	KeyDecisions    []DecisionRecord `json:"key_decisions"`
	PatternsDiscovered []string      `json:"patterns_discovered"`
	FilesModified   []string         `json:"files_modified"`
	SuggestionsAdopted int           `json:"suggestions_adopted"`
	TotalTurns      int              `json:"total_turns"`
	Duration        time.Duration    `json:"duration"`
	DistilledAt     time.Time        `json:"distilled_at"`
}

// TraceSession represents local trace file layout on disk.
type TraceSession struct {
	SessionID   string          `json:"session_id"`
	ActiveBranch string         `json:"active_branch"`
	Branches    []DecisionBranch `json:"branches"`
	Summary     *DistillSummary  `json:"summary,omitempty"`
	LastUpdated time.Time       `json:"last_updated"`
}
