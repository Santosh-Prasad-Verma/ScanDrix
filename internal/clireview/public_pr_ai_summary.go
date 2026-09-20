package clireview

import (
	"context"

	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// PublicPrAiSummaryService produces concise AI summaries of public pull requests.
type PublicPrAiSummaryService = adapters.PublicPrAiSummaryService

// NewPublicPrAiSummaryService creates a new summary service.
func NewPublicPrAiSummaryService(runner func(ctx context.Context, prompt string) (string, error)) *PublicPrAiSummaryService {
	return adapters.NewPublicPrAiSummaryService(runner)
}
