// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_set.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// SetRuleLikeUseCase registers an upvote or downvote on a library rule.
type SetRuleLikeUseCase struct {
	ruleLikeService contracts.IRuleLikeService
}

// NewSetRuleLikeUseCase constructs the use case.
func NewSetRuleLikeUseCase(ruleLikeService contracts.IRuleLikeService) *SetRuleLikeUseCase {
	return &SetRuleLikeUseCase{ruleLikeService: ruleLikeService}
}

// Execute persists or updates a user's feedback for a rule.
func (uc *SetRuleLikeUseCase) Execute(
	ctx context.Context,
	ruleID string,
	feedback entities.RuleFeedbackType,
	userID string,
) (*entities.RuleLikeEntity, error) {
	if ruleID == "" {
		return nil, fmt.Errorf("rule ID is required")
	}

	res, err := uc.ruleLikeService.SetFeedback(ctx, ruleID, feedback, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to set rule feedback: %w", err)
	}

	return res, nil
}
