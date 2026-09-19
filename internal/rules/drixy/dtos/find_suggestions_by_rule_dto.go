// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_suggestions_by_rule_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// FindSuggestionsByRuleDTO specifies the target rule identifier for finding related review suggestions.
type FindSuggestionsByRuleDTO struct {
	RuleID string `json:"ruleId"`
}
