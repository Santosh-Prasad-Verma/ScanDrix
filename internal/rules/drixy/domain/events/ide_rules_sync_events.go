// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: ide_rules_sync_events.go
// ═══════════════════════════════════════════════════════════════

package events

import "time"

// EventName constants for rule events.
const (
	EventIDERulesSynced              = "drixy_rules.ide_synced"
	EventGlobalRulesSynced           = "drixy_rules.global_synced"
	EventRulesGenerated              = "drixy_rules.generated"
	EventFileReferencesInvalidated   = "drixy_rules.file_references_invalid"
)

// IDERulesSyncEventPayload carries telemetry for IDE rules synchronization.
type IDERulesSyncEventPayload struct {
	OrganizationID string    `json:"organizationId"`
	TeamID         string    `json:"teamId"`
	RepositoryID   string    `json:"repositoryId"`
	RepositoryName string    `json:"repositoryName"`
	TotalRules     int       `json:"totalRules"`
	SourcePath     string    `json:"sourcePath,omitempty"`
	SyncedAt       time.Time `json:"syncedAt"`
}
