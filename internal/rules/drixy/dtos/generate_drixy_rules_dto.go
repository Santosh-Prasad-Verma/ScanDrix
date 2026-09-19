// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: generate_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// GenerateDrixyRulesDTO specifies time range and repository filters for past PR learning.
type GenerateDrixyRulesDTO struct {
	TeamID          string   `json:"teamId"`
	Months          int      `json:"months,omitempty"`
	Weeks           int      `json:"weeks,omitempty"`
	Days            int      `json:"days,omitempty"`
	RepositoriesIDs []string `json:"repositoriesIds,omitempty"`
	Comments        []string `json:"comments,omitempty"`
}
