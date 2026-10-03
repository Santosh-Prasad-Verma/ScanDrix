// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_service.go
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// IRuleLikeService provides business logic for community and catalog rule feedback.
type IRuleLikeService interface {
	SetFeedback(ctx context.Context, organizationID, ruleID string, feedback entities.RuleFeedbackType, userID string) (*entities.RuleLikeEntity, error)
	CountByRule(ctx context.Context, organizationID, ruleID string) (int, error)
	TopByLanguage(ctx context.Context, language string, limit int) ([]map[string]any, error)
	GetAllRulesWithFeedback(ctx context.Context, organizationID, userID string) ([]RuleFeedbackSummary, error)
	RemoveFeedback(ctx context.Context, organizationID, ruleID, userID string) (bool, error)
}
