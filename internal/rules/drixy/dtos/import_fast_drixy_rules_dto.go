// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: import_fast_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// FastSyncRuleItem models an individual rule detected during fast IDE scanning.
type FastSyncRuleItem struct {
	Title        string                         `json:"title"`
	Rule         string                         `json:"rule"`
	Path         string                         `json:"path,omitempty"`
	SourcePath   string                         `json:"sourcePath,omitempty"`
	Severity     string                         `json:"severity,omitempty"`
	Scope        interfaces.DrixyRulesScope     `json:"scope,omitempty"`
	RepositoryID string                         `json:"repositoryId"`
	Examples     []interfaces.DrixyRulesExample `json:"examples,omitempty"`
	PathSource   string                         `json:"pathSource,omitempty"`
}

// ImportFastDrixyRulesDto provides payload for importing rules extracted via fast sync.
type ImportFastDrixyRulesDto struct {
	TeamID string             `json:"teamId"`
	Rules  []FastSyncRuleItem `json:"rules"`
}

// ReviewFastDrixyRulesDto allows approving or discarding fast imported rules in batch.
type ReviewFastDrixyRulesDto struct {
	ActivateRuleIDs []string `json:"activateRuleIds,omitempty"`
	DeleteRuleIDs   []string `json:"deleteRuleIds,omitempty"`
}
