package domain

import (
	"time"

	"github.com/google/uuid"
)

// Automation defines custom webhook actions, auto-fixes, and triggerable workflows.
type Automation struct {
	TenantScopedEntity
	OrganizationID      uuid.UUID `json:"organization_id" db:"organization_id"`
	Name                string    `json:"name" db:"name"`
	TriggerEvent        string    `json:"trigger_event" db:"trigger_event"`
	ActionType          string    `json:"action_type" db:"action_type"`
	FilterConditions    JSONBMap  `json:"filter_conditions" db:"filter_conditions"`
	ActionConfiguration JSONBMap  `json:"action_configuration" db:"action_configuration"`
	IsActive            bool      `json:"is_active" db:"is_active"`
}

// AutomationModel alias.
type AutomationModel = Automation

// AutomationExecution records execution traces for triggered automations.
type AutomationExecution struct {
	TenantScopedEntity
	AutomationID   uuid.UUID `json:"automation_id" db:"automation_id"`
	TriggerPayload JSONBMap  `json:"trigger_payload" db:"trigger_payload"`
	Status         string    `json:"status" db:"status"`
	Logs           *string   `json:"logs,omitempty" db:"logs"`
	DurationMs     int64     `json:"duration_ms" db:"duration_ms"`
	ExecutedAt     time.Time `json:"executed_at" db:"executed_at"`
}

// AutomationExecutionModel alias.
type AutomationExecutionModel = AutomationExecution

// TeamAutomation maps automations to specific squads.
type TeamAutomation struct {
	TenantScopedEntity
	TeamID       uuid.UUID `json:"team_id" db:"team_id"`
	AutomationID uuid.UUID `json:"automation_id" db:"automation_id"`
}

// TeamAutomationModel alias.
type TeamAutomationModel = TeamAutomation
