// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_ids_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// RuleIdsDto identifies multiple rules for batch operations.
type RuleIdsDto struct {
	RuleIDs []string `json:"ruleIds"`
	TeamID  string   `json:"teamId,omitempty"`
}
