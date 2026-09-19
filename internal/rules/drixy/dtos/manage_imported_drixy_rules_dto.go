// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: manage_imported_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// ManageImportedRulesAction defines bulk lifecycle actions on imported repository rules.
type ManageImportedRulesAction string

const (
	ManageImportedRulesActionPause  ManageImportedRulesAction = "pause"
	ManageImportedRulesActionResume ManageImportedRulesAction = "resume"
	ManageImportedRulesActionDelete ManageImportedRulesAction = "delete"
)

// ManageImportedDrixyRulesDto defines payload for bulk managing repository imported rules.
type ManageImportedDrixyRulesDto struct {
	RepositoryID string                    `json:"repositoryId"`
	Action       ManageImportedRulesAction `json:"action"`
}

// ManageImportedRulesResult summarizes count of affected rules.
type ManageImportedRulesResult struct {
	Action ManageImportedRulesAction `json:"action"`
	Counts struct {
		Active  int `json:"active"`
		Paused  int `json:"paused"`
		Deleted int `json:"deleted"`
		Pinned  int `json:"pinned"`
	} `json:"counts"`
}
