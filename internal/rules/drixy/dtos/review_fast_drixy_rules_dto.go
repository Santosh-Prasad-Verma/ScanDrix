// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: review_fast_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// ReviewFastDrixyRulesDTO models batch approval and deletion of fast-synced rules.
type ReviewFastDrixyRulesDTO struct {
	TeamID          string   `json:"teamId"`
	ActivateRuleIDs []string `json:"activateRuleIds,omitempty"`
	DeleteRuleIDs   []string `json:"deleteRuleIds,omitempty"`
}
