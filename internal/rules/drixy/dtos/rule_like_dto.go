// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// SetRuleFeedbackDto captures user vote payload.
type SetRuleFeedbackDto struct {
	Feedback entities.RuleFeedbackType `json:"feedback"`
}

// RuleLikeResponse provides feedback state for a rule.
type RuleLikeResponse struct {
	RuleID        string                     `json:"ruleId"`
	PositiveCount int                        `json:"positiveCount"`
	NegativeCount int                        `json:"negativeCount"`
	UserFeedback  *entities.RuleFeedbackType `json:"userFeedback,omitempty"`
}
