package sync

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// RuleDefinition represents a declarative rule entry in .scandrix/rules.yaml or .scandrix/rules.json.
type RuleDefinition struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	PathPattern string                 `json:"path_pattern" yaml:"path_pattern"`
	RegexRule   string                 `json:"regex_rule" yaml:"regex_rule"`
	Severity    models.FindingSeverity `json:"severity" yaml:"severity"`
	Category    string                 `json:"category" yaml:"category"`
	Description string                 `json:"description" yaml:"description"`
	Remediation string                 `json:"remediation" yaml:"remediation"`
	Tags        []string               `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// RuleConfigFile represents the top-level schema of a repository or workspace rules file.
type RuleConfigFile struct {
	Version string           `json:"version" yaml:"version"`
	Rules   []RuleDefinition `json:"rules" yaml:"rules"`
}

// SyncResult details the outcome of synchronizing declarative rules into the active evaluator.
type SyncResult struct {
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	TotalParsed  int       `json:"total_parsed"`
	AddedCount   int       `json:"added_count"`
	UpdatedCount int       `json:"updated_count"`
	InvalidCount int       `json:"invalid_count"`
	Errors       []string  `json:"errors,omitempty"`
	SyncedAt     time.Time `json:"synced_at"`
}

// SweepRequest triggers an on-demand security and quality scan of an entire repository branch.
type SweepRequest struct {
	SweepID       uuid.UUID `json:"sweep_id"`
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	RepositoryID  uuid.UUID `json:"repository_id"`
	RepoNamespace string    `json:"repo_namespace"`
	Branch        string    `json:"branch"`
	MaxFiles      int       `json:"max_files,omitempty"`
}

// SweepReport aggregates findings and metrics across a full codebase sweep.
type SweepReport struct {
	SweepID          uuid.UUID            `json:"sweep_id"`
	WorkspaceID      uuid.UUID            `json:"workspace_id"`
	RepoNamespace    string               `json:"repo_namespace"`
	Branch           string               `json:"branch"`
	FilesScanned     int                  `json:"files_scanned"`
	LinesProcessed   int64                `json:"lines_processed"`
	FindingsCount    int                  `json:"findings_count"`
	CriticalCount    int                  `json:"critical_count"`
	HighCount        int                  `json:"high_count"`
	MediumCount      int                  `json:"medium_count"`
	LowCount         int                  `json:"low_count"`
	Duration         time.Duration        `json:"duration"`
	Findings         []models.CodeFinding `json:"findings,omitempty"`
	CompletedAt      time.Time            `json:"completed_at"`
}

// RulePerformance tracks the effectiveness and false-positive rate of an individual rule.
type RulePerformance struct {
	RuleID         uuid.UUID     `json:"rule_id"`
	RuleName       string        `json:"rule_name"`
	TriggerCount   int           `json:"trigger_count"`
	FalsePositives int           `json:"false_positives"`
	AccuracyRate   float64       `json:"accuracy_rate"`
	AverageLatency time.Duration `json:"average_latency"`
}
