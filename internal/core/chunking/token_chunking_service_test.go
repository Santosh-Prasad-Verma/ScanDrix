package chunking_test

import (
	"testing"

	"github.com/scandrix/backend/internal/core/chunking"
	"github.com/stretchr/testify/assert"
)

func TestTokenChunkingServiceEmpty(t *testing.T) {
	svc := chunking.NewTokenChunkingService()
	res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Data: nil,
	})

	assert.Equal(t, 0, res.TotalItems)
	assert.Equal(t, 0, res.TotalChunks)
	assert.Empty(t, res.Chunks)
}

func TestTokenChunkingServicePartitioning(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	// 10 items of 200 characters each (~57 tokens each)
	items := make([]any, 10)
	sample := "function calculateReviewScore(diff: string): number { return diff.length * 42; }"
	for i := 0; i < 10; i++ {
		items[i] = sample
	}

	// Limit to ~100 tokens per chunk with 60% usage
	res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Model:             "claude-3-5-sonnet",
		Data:              items,
		UsagePercentage:   50,
		OverrideMaxTokens: 200, // tokenLimit = 100
	})

	assert.Equal(t, 10, res.TotalItems)
	assert.True(t, res.TotalChunks > 1)
	assert.Equal(t, res.TotalChunks, len(res.Chunks))
	assert.Equal(t, 100, res.TokenLimit)

	totalPartitioned := 0
	for _, c := range res.Chunks {
		totalPartitioned += len(c)
	}
	assert.Equal(t, 10, totalPartitioned)
}

func TestGetMaxTokensForModel(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	assert.Equal(t, 1000000, svc.GetMaxTokensForModel("gemini-2.5-pro", 0))
	assert.Equal(t, 200000, svc.GetMaxTokensForModel("claude-3-5-sonnet", 0))
	assert.Equal(t, 128000, svc.GetMaxTokensForModel("gpt-4o", 0))
	assert.Equal(t, 64000, svc.GetMaxTokensForModel("unknown-model", 0))
}
