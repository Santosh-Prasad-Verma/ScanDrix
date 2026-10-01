// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: change_status_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// ChangeStatusDrixyRulesDTO alters status across a list of rules.
type ChangeStatusDrixyRulesDTO struct {
	RuleIDs []string                    `json:"ruleIds"`
	Status  interfaces.DrixyRulesStatus `json:"status"`
	TeamID  string                      `json:"teamId,omitempty"`
}
