package domain

import (
	"time"
)

// AutomationEntity represents a configured automation template or capability.
type AutomationEntity struct {
	UUID           string          `json:"uuid"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Tags           []string        `json:"tags"`
	AntiPatterns   []string        `json:"anti_patterns"`
	AutomationType AutomationType  `json:"automation_type"`
	Status         bool            `json:"status"`
	Level          AutomationLevel `json:"level"`
}

// AutomationExecutionEntity tracks an individual execution of an automation workflow.
type AutomationExecutionEntity struct {
	UUID                 string                       `json:"uuid"`
	CreatedAt            time.Time                    `json:"created_at"`
	UpdatedAt            time.Time                    `json:"updated_at"`
	Status               AutomationStatus             `json:"status"`
	ErrorMessage         string                       `json:"error_message,omitempty"`
	DataExecution        map[string]any               `json:"data_execution,omitempty"`
	PullRequestNumber    int                          `json:"pull_request_number,omitempty"`
	RepositoryID         string                       `json:"repository_id,omitempty"`
	TeamAutomation       *TeamAutomationEntity        `json:"team_automation,omitempty"`
	CodeReviewExecutions []*CodeReviewExecutionEntity `json:"code_review_executions,omitempty"`
	Origin               string                       `json:"origin,omitempty"`
}

// CodeReviewExecutionEntity records granular stage transitions within a review pipeline.
type CodeReviewExecutionEntity struct {
	UUID                 string                     `json:"uuid"`
	CreatedAt            time.Time                  `json:"created_at"`
	UpdatedAt            time.Time                  `json:"updated_at"`
	AutomationExecution  *AutomationExecutionEntity `json:"automation_execution,omitempty"`
	AutomationExecutionID string                    `json:"automation_execution_id,omitempty"`
	Status               AutomationStatus           `json:"status"`
	StageName            string                     `json:"stage_name,omitempty"`
	Message              string                     `json:"message,omitempty"`
	Metadata             map[string]any             `json:"metadata,omitempty"`
	FinishedAt           *time.Time                 `json:"finished_at,omitempty"`
}

// TeamAutomationEntity binds an automation type to a specific team.
type TeamAutomationEntity struct {
	UUID         string            `json:"uuid"`
	Status       bool              `json:"status"`
	Automation   *AutomationEntity `json:"automation,omitempty"`
	AutomationID string            `json:"automation_id,omitempty"`
	TeamID       string            `json:"team_id,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
