// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_remove.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
)

// RemoveRuleLikeUseCase revokes a user's feedback for a rule.
type RemoveRuleLikeUseCase struct {
	ruleLikeService contracts.IRuleLikeService
}

// NewRemoveRuleLikeUseCase constructs the remove use case.
func NewRemoveRuleLikeUseCase(ruleLikeService contracts.IRuleLikeService) *RemoveRuleLikeUseCase {
	return &RemoveRuleLikeUseCase{ruleLikeService: ruleLikeService}
}

// Execute removes previously submitted feedback.
func (uc *RemoveRuleLikeUseCase) Execute(
	ctx context.Context,
	organizationID string,
	ruleID string,
	userID string,
) (bool, error) {
	if ruleID == "" {
		return false, fmt.Errorf("rule ID is required")
	}
	if userID == "" {
		return false, fmt.Errorf("user ID is required to remove feedback")
	}

	return uc.ruleLikeService.RemoveFeedback(ctx, organizationID, ruleID, userID)
}
