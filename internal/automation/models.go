package automation

import (
	"time"

	"github.com/google/uuid"
)

// TriggerType defines pull request events that activate automation workflows.
type TriggerType string

const (
	TriggerPROpened      TriggerType = "PR_OPENED"
	TriggerPRSynchronize TriggerType = "PR_SYNCHRONIZE"
	TriggerPRMerged      TriggerType = "PR_MERGED"
)

// ActionType enumerates operations executed when automation conditions match.
type ActionType string

const (
	ActionTriggerReview ActionType = "TRIGGER_REVIEW"
	ActionAutoAssign    ActionType = "AUTO_ASSIGN"
	ActionAddLabel      ActionType = "ADD_LABEL"
	ActionPostSummary   ActionType = "POST_SUMMARY"
)

// RuleCondition specifies filters that must be satisfied for a rule to trigger.
type RuleCondition struct {
	BranchPattern string   `json:"branch_pattern,omitempty"` // e.g. "release/*", "main"
	PathGlobs     []string `json:"path_globs,omitempty"`     // e.g. ["auth/**", "*.sql"]
	AuthorRegex   string   `json:"author_regex,omitempty"`
}

// RuleAction defines a specific task executed when an automation rule matches.
type RuleAction struct {
	Type       ActionType        `json:"type"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// AutomationRule models a user-configurable team workflow.
type AutomationRule struct {
	ID          uuid.UUID     `json:"id" db:"id"`
	WorkspaceID uuid.UUID     `json:"workspace_id" db:"workspace_id"`
	Name        string        `json:"name" db:"name"`
	Enabled     bool          `json:"enabled" db:"enabled"`
	Trigger     TriggerType   `json:"trigger" db:"trigger"`
	Conditions  RuleCondition `json:"conditions" db:"conditions"`
	Actions     []RuleAction  `json:"actions" db:"actions"`
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at" db:"updated_at"`
}

// TriggerContext provides pull request metadata to the evaluation engine.
type TriggerContext struct {
	WorkspaceID  uuid.UUID
	Trigger      TriggerType
	TargetBranch string
	Author       string
	ChangedFiles []string
	PullNumber   int
}

// ExecutedAction records an action carried out during automation execution.
type ExecutedAction struct {
	RuleID     uuid.UUID         `json:"rule_id"`
	RuleName   string            `json:"rule_name"`
	ActionType ActionType        `json:"action_type"`
	Parameters map[string]string `json:"parameters"`
	ExecutedAt time.Time         `json:"executed_at"`
}
