package diagnostics

import (
	"time"

	"github.com/google/uuid"
)

// HealthStatus classifies component readiness.
type HealthStatus string

const (
	StatusHealthy   HealthStatus = "HEALTHY"
	StatusDegraded  HealthStatus = "DEGRADED"
	StatusUnhealthy HealthStatus = "UNHEALTHY"
)

// ComponentType identifies the infrastructure category.
type ComponentType string

const (
	ComponentDatabase ComponentType = "DATABASE"
	ComponentQueue    ComponentType = "QUEUE"
	ComponentCache    ComponentType = "CACHE"
	ComponentLLM      ComponentType = "LLM_PROVIDER"
	ComponentStorage  ComponentType = "STORAGE"
)

// ComponentHealth encapsulates diagnostics for an individual subsystem.
type ComponentHealth struct {
	Name        string         `json:"name"`
	Type        ComponentType  `json:"type"`
	Status      HealthStatus   `json:"status"`
	Latency     time.Duration  `json:"latency"`
	Message     string         `json:"message"`
	Details     map[string]any `json:"details,omitempty"`
	LastChecked time.Time      `json:"last_checked"`
}

// SystemDiagnosticReport provides an overall cluster readiness audit.
type SystemDiagnosticReport struct {
	OverallStatus     HealthStatus               `json:"overall_status"`
	Components        map[string]ComponentHealth `json:"components"`
	Uptime            time.Duration              `json:"uptime"`
	MemoryAllocatedMB float64                    `json:"memory_allocated_mb"`
	Goroutines        int                        `json:"goroutines"`
	LastChecked       time.Time                  `json:"last_checked"`
}

// ActionType enumerates automated remediation actions.
type ActionType string

const (
	ActionCacheEviction ActionType = "CACHE_EVICTION"
	ActionDLQRedrive    ActionType = "DLQ_REDRIVE"
	ActionCircuitReset  ActionType = "CIRCUIT_RESET"
	ActionLeaseReclaim  ActionType = "LEASE_RECLAIM"
)

// SelfHealingAction logs an automated correction performed by the daemon.
type SelfHealingAction struct {
	ID              uuid.UUID  `json:"id"`
	ActionType      ActionType `json:"action_type"`
	TargetComponent string     `json:"target_component"`
	TriggerReason   string     `json:"trigger_reason"`
	ExecutedAt      time.Time  `json:"executed_at"`
	Success         bool       `json:"success"`
	OutcomeMessage  string     `json:"outcome_message"`
}
