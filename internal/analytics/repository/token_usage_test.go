package repository

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/analytics/pricing"
	"github.com/scandrix/backend/internal/analytics/usage"
	"github.com/scandrix/backend/internal/core/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageRepository_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	cat := pricing.NewTokenPricingCatalog(pricing.WithOnlineFetching(false))
	resolver := pricing.NewPricingResolver(cat)
	repo := NewTokenUsageRepository(resolver)

	now := time.Now().UTC()

	// Ingest span 1: PR 10, review area, Claude 3.5 Sonnet
	err := repo.IngestSpan(ctx, RawSpanRecord{
		CorrelationID:  "rev-1",
		OrganizationID: "org-1",
		PRNumber:       10,
		Timestamp:      now.Add(-2 * time.Hour),
		TokenUsage: log.TokenUsageTu{
			IsByok:       true,
			Model:        "claude-3-5-sonnet",
			CredentialID: "cred-anthropic-1",
			Input:        1000,
			Output:       200,
			Total:        1200,
			Reasoning:    0,
			Area:         log.AreaReview,
			Route:        "codeReview",
		},
	})
	require.NoError(t, err)

	// Ingest span 2: PR 10, suggestions area, Claude 3.5 Sonnet
	err = repo.IngestSpan(ctx, RawSpanRecord{
		CorrelationID:  "rev-1",
		OrganizationID: "org-1",
		PRNumber:       10,
		Timestamp:      now.Add(-1 * time.Hour),
		TokenUsage: log.TokenUsageTu{
			IsByok:       true,
			Model:        "claude-3-5-sonnet",
			CredentialID: "cred-anthropic-1",
			Input:        500,
			Output:       100,
			Total:        600,
			Reasoning:    0,
			Area:         log.AreaSuggestions,
			Route:        "codeReview",
		},
	})
	require.NoError(t, err)

	// Ingest span 3: PR 12, summary area, GPT-4o
	err = repo.IngestSpan(ctx, RawSpanRecord{
		CorrelationID:  "rev-2",
		OrganizationID: "org-1",
		PRNumber:       12,
		Timestamp:      now,
		TokenUsage: log.TokenUsageTu{
			IsByok:       true,
			Model:        "gpt-4o",
			CredentialID: "cred-openai-1",
			Input:        2000,
			Output:       400,
			Total:        2400,
			Reasoning:    0,
			Area:         log.AreaSummary,
			Route:        "prSummary",
		},
	})
	require.NoError(t, err)

	q := usage.TokenUsageQueryContract{
		OrganizationID: "org-1",
		Start:          now.Add(-24 * time.Hour),
		End:            now.Add(1 * time.Hour),
		BYOK:           true,
	}

	// 1. GetSummary
	summary, err := repo.GetSummary(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, int64(3500), summary.Input)
	assert.Equal(t, int64(700), summary.Output)
	assert.Equal(t, int64(4200), summary.Total)

	// 2. GetSummaryByModel
	byModel, err := repo.GetSummaryByModel(ctx, q)
	require.NoError(t, err)
	require.Len(t, byModel, 2)
	assert.Equal(t, "claude-3-5-sonnet", byModel[0].Model)
	assert.Equal(t, int64(1500), byModel[0].Input)
	assert.Equal(t, "gpt-4o", byModel[1].Model)
	assert.Equal(t, int64(2000), byModel[1].Input)

	// 3. GetModelCredentialPairs
	pairs, err := repo.GetModelCredentialPairs(ctx, q)
	require.NoError(t, err)
	require.Len(t, pairs, 2)

	// 4. GetUsageByPr
	byPr, err := repo.GetUsageByPr(ctx, q)
	require.NoError(t, err)
	require.Len(t, byPr, 2)
	assert.Equal(t, 10, byPr[0].PRNumber)
	assert.Equal(t, int64(1500), byPr[0].Input)
	assert.Equal(t, 12, byPr[1].PRNumber)
	assert.Equal(t, int64(2000), byPr[1].Input)

	// 5. GetUsageOverview
	overview, err := repo.GetUsageOverview(ctx, q)
	require.NoError(t, err)
	assert.Equal(t, int64(4200), overview.Summary.Totals.Total)
	require.Len(t, overview.ByPr, 2)
	require.Len(t, overview.ByArea, 3)
	require.Len(t, overview.ByTaskArea, 3)
	require.Len(t, overview.ByTaskModelSpan, 2)
}
