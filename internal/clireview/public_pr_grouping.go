package clireview

import (
	"context"

	"github.com/scandrix/backend/internal/clireview/infrastructure/adapters"
)

// PublicPrGroupingService clusters changed files by architectural intent.
type PublicPrGroupingService = adapters.PublicPrGroupingService

// NewPublicPrGroupingService creates a new grouping service.
func NewPublicPrGroupingService(runner func(ctx context.Context, prompt string) (string, error)) *PublicPrGroupingService {
	return adapters.NewPublicPrGroupingService(runner)
}
