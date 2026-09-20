package repository

import (
	"context"

	"github.com/scandrix/backend/internal/analytics/usage"
)

// TokenUsageService wraps TokenUsageRepository implementing usage.ITokenUsageService.
type TokenUsageService struct {
	repo *TokenUsageRepository
}

// NewTokenUsageService creates a new token usage service.
func NewTokenUsageService(repo *TokenUsageRepository) *TokenUsageService {
	return &TokenUsageService{repo: repo}
}

func (s *TokenUsageService) GetSummary(ctx context.Context, query usage.TokenUsageQueryContract) (usage.UsageSummaryContract, error) {
	return s.repo.GetSummary(ctx, query)
}

func (s *TokenUsageService) GetSummaryByModel(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.BaseUsageContract, error) {
	return s.repo.GetSummaryByModel(ctx, query)
}

func (s *TokenUsageService) GetDailyUsage(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.DailyUsageResultContract, error) {
	return s.repo.GetDailyUsage(ctx, query)
}

func (s *TokenUsageService) GetModelCredentialPairs(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.ModelCredentialPair, error) {
	return s.repo.GetModelCredentialPairs(ctx, query)
}

func (s *TokenUsageService) GetUsageByPr(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.UsageByPrResultContract, error) {
	return s.repo.GetUsageByPr(ctx, query)
}

func (s *TokenUsageService) GetDailyUsageByPr(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.DailyUsageByPrResultContract, error) {
	return s.repo.GetDailyUsageByPr(ctx, query)
}

func (s *TokenUsageService) GetUsageByReview(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.UsageByReviewResultContract, error) {
	return s.repo.GetUsageByReview(ctx, query)
}

func (s *TokenUsageService) GetUsageByArea(ctx context.Context, query usage.TokenUsageQueryContract) ([]usage.UsageByAreaResultContract, error) {
	return s.repo.GetUsageByArea(ctx, query)
}

func (s *TokenUsageService) GetUsageOverview(ctx context.Context, query usage.TokenUsageQueryContract) (*usage.UsageOverviewReportContract, error) {
	return s.repo.GetUsageOverview(ctx, query)
}
