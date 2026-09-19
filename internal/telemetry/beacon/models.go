// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Telemetry Subsystem
// Package: beacon
// File: models.go
// ═══════════════════════════════════════════════════════════════

package beacon

// KnownIntegrations is the canonical whitelist of integration platform slugs.
var KnownIntegrations = map[string]struct{}{
	"github":       {},
	"gitlab":       {},
	"bitbucket":    {},
	"azure_repos":  {},
	"azure_boards": {},
	"jira":         {},
	"slack":        {},
	"msteams":      {},
	"discord":      {},
	"notion":       {},
	"forgejo":      {},
}

// DeploymentType represents the detected container / virtualization environment.
type DeploymentType string

const (
	DeploymentDocker        DeploymentType = "docker"
	DeploymentDockerCompose DeploymentType = "docker-compose"
	DeploymentKubernetes    DeploymentType = "k8s"
	DeploymentBare          DeploymentType = "bare"
	DeploymentUnknown       DeploymentType = "unknown"
)

// ScanDrixInfo encapsulates application runtime release and deployment mode metadata.
type ScanDrixInfo struct {
	Version     string         `json:"version"`
	Deployment  DeploymentType `json:"deployment"`
	UptimeHours int64          `json:"uptime_hours"`
}

// RuntimeInfo describes the host system, architecture, and database engine.
type RuntimeInfo struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"` // "linux" | "darwin" | "windows"
	Arch      string `json:"arch"`
	CPUCount  int    `json:"cpu_count"`
	DBType    string `json:"db_type"` // "postgres"
	DBVersion string `json:"db_version"`
}

// Usage7d captures rolling 7-day high-level aggregated operational volume.
type Usage7d struct {
	ActiveUsers          int64 `json:"active_users"`
	Organizations        int64 `json:"organizations"`
	Teams                int64 `json:"teams"`
	ReposConnected       int64 `json:"repos_connected"`
	PRsReviewed          int64 `json:"prs_reviewed"`
	SuggestionsGenerated int64 `json:"suggestions_generated"`
	SuggestionsApplied   int64 `json:"suggestions_applied"`
}

// ConfigSummary outlines feature enablement and integrated third-party ecosystem hooks.
type ConfigSummary struct {
	DrixyRulesEnabled   bool     `json:"drixy_rules_enabled"`
	AgentReviewReposPct int64    `json:"agent_review_repos_pct"`
	Integrations        []string `json:"integrations"`
}

// HeartbeatMetrics mirrors the self-hosted beacon v1 schema (excluding envelope fields).
type HeartbeatMetrics struct {
	ScanDrix ScanDrixInfo   `json:"scandrix"`
	Runtime  RuntimeInfo    `json:"runtime"`
	Usage7d  Usage7d        `json:"usage_7d"`
	Config   ConfigSummary  `json:"config"`
}

// TelemetryStateValue represents the persisted singleton heartbeat coordinator state.
type TelemetryStateValue struct {
	InstanceID        string  `json:"instance_id"`
	FirstSeenAt       string  `json:"first_seen_at"` // ISO-8601 UTC
	LastSentDay       *string `json:"last_sent_day"` // YYYY-MM-DD UTC
	InFlightDay       *string `json:"in_flight_day"` // YYYY-MM-DD UTC
	InFlightStartedAt *string `json:"in_flight_started_at"` // ISO-8601 UTC
}

// CollectInput provides instance inception details required to compute relative uptime.
type CollectInput struct {
	FirstSeenAt string // ISO-8601 UTC
}
