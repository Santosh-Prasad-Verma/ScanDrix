// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_repository.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// RuleFeedbackSummary aggregates voting counts per rule.
type RuleFeedbackSummary struct {
	RuleID        string                    `json:"ruleId"`
	PositiveCount int                       `json:"positiveCount"`
	NegativeCount int                       `json:"negativeCount"`
	UserFeedback  *entities.RuleFeedbackType `json:"userFeedback"`
}

// IRuleLikeRepository handles feedback persistence and aggregates.
type IRuleLikeRepository interface {
	SetFeedback(ctx context.Context, ruleID string, feedback entities.RuleFeedbackType, userID string) (*entities.RuleLikeEntity, error)
	FindOne(ctx context.Context, ruleID, userID string) (*entities.RuleLikeEntity, error)
	CountByRule(ctx context.Context, ruleID string) (int, error)
	TopByLanguage(ctx context.Context, language string, limit int) ([]map[string]any, error)
	GetAllRulesWithFeedback(ctx context.Context, userID string) ([]RuleFeedbackSummary, error)
	Unlike(ctx context.Context, ruleID, userID string) (bool, error)
}
