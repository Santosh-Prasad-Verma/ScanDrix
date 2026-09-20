// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: rule_like_service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/entities"
)

// RuleLikeService provides business logic for community feedback.
type RuleLikeService struct {
	repo contracts.IRuleLikeRepository
}

// NewRuleLikeService initializes the service.
func NewRuleLikeService(repo contracts.IRuleLikeRepository) *RuleLikeService {
	return &RuleLikeService{repo: repo}
}

// SetFeedback inserts or updates feedback for a rule.
func (s *RuleLikeService) SetFeedback(ctx context.Context, ruleID string, feedback entities.RuleFeedbackType, userID string) (*entities.RuleLikeEntity, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.SetFeedback(ctx, ruleID, feedback, userID)
}

// CountByRule returns positive vote count for a rule.
func (s *RuleLikeService) CountByRule(ctx context.Context, ruleID string) (int, error) {
	if s.repo == nil {
		return 0, nil
	}
	return s.repo.CountByRule(ctx, ruleID)
}

// TopByLanguage retrieves top ranked rules by language.
func (s *RuleLikeService) TopByLanguage(ctx context.Context, language string, limit int) ([]map[string]any, error) {
	if s.repo == nil {
		return []map[string]any{}, nil
	}
	return s.repo.TopByLanguage(ctx, language, limit)
}

// GetAllRulesWithFeedback aggregates feedback for all rules.
func (s *RuleLikeService) GetAllRulesWithFeedback(ctx context.Context, userID string) ([]contracts.RuleFeedbackSummary, error) {
	if s.repo == nil {
		return []contracts.RuleFeedbackSummary{}, nil
	}
	return s.repo.GetAllRulesWithFeedback(ctx, userID)
}

// RemoveFeedback removes feedback from a rule.
func (s *RuleLikeService) RemoveFeedback(ctx context.Context, ruleID, userID string) (bool, error) {
	if s.repo == nil {
		return false, nil
	}
	return s.repo.Unlike(ctx, ruleID, userID)
}
