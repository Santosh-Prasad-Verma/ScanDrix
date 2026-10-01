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
	RuleID        string                     `json:"ruleId"`
	PositiveCount int                        `json:"positiveCount"`
	NegativeCount int                        `json:"negativeCount"`
	UserFeedback  *entities.RuleFeedbackType `json:"userFeedback"`
}

// IRuleLikeRepository handles feedback persistence and aggregates.
// organizationID is required on every tenant-scoped call so the repository can
// open a transaction with the RLS tenant context set. Without it the policy in
// migration 040 admits zero rows and every write is rejected, which would look
// identical to "this workspace has no feedback".
//
// TopByLanguage is deliberately exempt: it is a global catalog leaderboard and
// has no single owning tenant. It runs as a system operation.
type IRuleLikeRepository interface {
	SetFeedback(ctx context.Context, organizationID, ruleID string, feedback entities.RuleFeedbackType, userID string) (*entities.RuleLikeEntity, error)
	FindOne(ctx context.Context, organizationID, ruleID, userID string) (*entities.RuleLikeEntity, error)
	CountByRule(ctx context.Context, organizationID, ruleID string) (int, error)
	TopByLanguage(ctx context.Context, language string, limit int) ([]map[string]any, error)
	GetAllRulesWithFeedback(ctx context.Context, organizationID, userID string) ([]RuleFeedbackSummary, error)
	Unlike(ctx context.Context, organizationID, ruleID, userID string) (bool, error)
}
