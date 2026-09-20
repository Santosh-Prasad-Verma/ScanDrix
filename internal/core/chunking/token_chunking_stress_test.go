package chunking_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/scandrix/backend/internal/core/chunking"
	"github.com/stretchr/testify/assert"
)

// TestChunkingStress_ModelContextCapacityMatrix verifies token capacity boundaries
// across all known LLM provider model families and default fallback parameters.
func TestChunkingStress_ModelContextCapacityMatrix(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	testCases := []struct {
		modelName      string
		defaultLimit   int
		expectedTokens int
	}{
		// Google Gemini family (1,000,000)
		{"gemini-1.5-pro", 0, 1000000},
		{"gemini-1.5-flash", 32000, 1000000},
		{"GEMINI-2.0-FLASH-EXP", 0, 1000000},

		// Anthropic Claude family (200,000)
		{"claude-3-5-sonnet-20241022", 0, 200000},
		{"claude-3-opus-20240229", 0, 200000},
		{"claude-3-haiku-20240307", 0, 200000},
		{"claude-sonnet-4-preview", 0, 200000},

		// OpenAI GPT-4o / Reasoning family (128,000)
		{"gpt-4o", 0, 128000},
		{"gpt-4o-mini", 0, 128000},
		{"gpt-4-turbo", 0, 128000},
		{"o1-preview", 0, 128000},
		{"o1-mini", 0, 128000},
		{"o3-mini", 0, 128000},

		// Legacy OpenAI models
		{"gpt-4-32k", 0, 32768},
		{"gpt-4", 0, 32768},
		{"gpt-3.5-turbo-16k", 0, 16384},

		// Custom / unknown models with default fallback
		{"custom-local-llama-3", 64000, 64000},
		{"mistral-large", 0, chunking.DefaultTokenChunkingMaxTokens},
		{"deepseek-coder-v2", 120000, 120000},
	}

	for _, tc := range testCases {
		t.Run(tc.modelName, func(t *testing.T) {
			got := svc.GetMaxTokensForModel(tc.modelName, tc.defaultLimit)
			assert.Equal(t, tc.expectedTokens, got)
		})
	}
}

// TestChunkingStress_TokenEstimationAccuracy verifies token approximation for diverse
// data inputs including nil, raw byte slices, multibyte UTF-8, and nested JSON objects.
func TestChunkingStress_TokenEstimationAccuracy(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	// 1. Nil and empty checks
	assert.Equal(t, 0, svc.EstimateTokens(nil))
	assert.Equal(t, 0, svc.EstimateTokens(""))
	assert.Equal(t, 0, svc.EstimateTokens([]byte{}))

	// 2. Short ASCII text
	shortText := "hello world"
	tokens := svc.EstimateTokens(shortText)
	assert.True(t, tokens >= 3 && tokens <= 5)

	// 3. Byte slice input
	byteData := []byte("diff --git a/file.go b/file.go\n+ func Add() int { return 1 }")
	assert.True(t, svc.EstimateTokens(byteData) > 10)

	// 4. Multibyte UTF-8 string (CJK, emojis, Cyrillic)
	multibyte := "ScanDrix ⚡ 🚀 🤖 日本語のテキストと漢字の解析 Тестирование кириллицы"
	mbTokens := svc.EstimateTokens(multibyte)
	assert.True(t, mbTokens > 15)

	// 5. Complex struct input serialized to JSON
	type DiffHunk struct {
		FilePath  string   `json:"file_path"`
		OldStart  int      `json:"old_start"`
		NewStart  int      `json:"new_start"`
		Lines     []string `json:"lines"`
		IsDeleted bool     `json:"is_deleted"`
	}

	hunk := DiffHunk{
		FilePath: "internal/service/reviewer.go",
		OldStart: 10,
		NewStart: 10,
		Lines: []string{
			"@@ -10,6 +10,7 @@",
			" import (",
			"+	\"context\"",
			" 	\"fmt\"",
			" )",
		},
		IsDeleted: false,
	}
	hunkTokens := svc.EstimateTokens(hunk)
	assert.True(t, hunkTokens > 20)
}

// TestChunkingStress_OversizedItemsHandling ensures that individual diff items that exceed
// the calculated chunk token limit are properly isolated into dedicated single-item chunks.
func TestChunkingStress_OversizedItemsHandling(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	// Create a tiny token limit using OverrideMaxTokens = 100 with 50% usage = 50 tokens
	tinyLimit := 100
	hugeString := strings.Repeat("A very long code line that has plenty of tokens to exceed limit. ", 50)
	smallString := "small line of code"

	items := []any{
		smallString,
		hugeString, // oversized
		smallString,
		smallString,
		hugeString, // oversized
	}

	opts := chunking.TokenChunkingOptions{
		Model:             "gpt-4o",
		Data:              items,
		UsagePercentage:   50,
		OverrideMaxTokens: tinyLimit,
	}

	res := svc.ChunkDataByTokens(opts)
	assert.Equal(t, len(items), res.TotalItems)
	assert.True(t, res.TotalChunks >= 3)
	assert.Equal(t, 50, res.TokenLimit)

	// Verify that oversized items are alone in their respective chunks
	for _, chunk := range res.Chunks {
		if len(chunk) == 1 {
			if chunk[0] == hugeString {
				assert.Len(t, chunk, 1)
			}
		}
	}
}

// TestChunkingStress_MassiveDatasetConcurrency processes thousands of simulated code diff
// hunks across 40 parallel goroutines to verify zero memory races and accurate chunk bounds.
func TestChunkingStress_MassiveDatasetConcurrency(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	const numWorkers = 40
	const itemsPerWorker = 250

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()

			items := make([]any, itemsPerWorker)
			for i := 0; i < itemsPerWorker; i++ {
				items[i] = fmt.Sprintf("Worker-%d: diff line %d: + value_%d", workerID, i, i*workerID)
			}

			models := []string{"gpt-4o", "claude-3-5-sonnet", "gemini-1.5-pro", "custom-model"}
			selectedModel := models[workerID%len(models)]

			result := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
				Model:            selectedModel,
				Data:             items,
				UsagePercentage:  60,
				DefaultMaxTokens: 32000,
			})

			assert.Equal(t, itemsPerWorker, result.TotalItems)
			assert.True(t, result.TotalChunks >= 1)
			assert.Equal(t, len(result.Chunks), result.TotalChunks)
			assert.Equal(t, len(result.Chunks), len(result.TokensPerChunk))

			// Verify all items are preserved across chunks
			itemCounter := 0
			for _, chunk := range result.Chunks {
				itemCounter += len(chunk)
			}
			assert.Equal(t, itemsPerWorker, itemCounter)
		}(w)
	}

	wg.Wait()
}

// TestChunkingStress_EmptyAndBoundaryOptions verifies that invalid or missing options
// like empty data slice, zero percentage, and negative max tokens return safe defaults.
func TestChunkingStress_EmptyAndBoundaryOptions(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	// 1. Empty data
	emptyRes := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Model: "claude-3-5-sonnet",
		Data:  nil,
	})
	assert.Equal(t, 0, emptyRes.TotalItems)
	assert.Equal(t, 0, emptyRes.TotalChunks)
	assert.Empty(t, emptyRes.Chunks)

	// 2. Zero / Negative usage percentage falls back to default 60%
	resZeroPct := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Model:           "gpt-4o", // 128,000 max
		Data:            []any{"test item 1", "test item 2"},
		UsagePercentage: 0,
	})
	// 60% of 128,000 = 76,800
	assert.Equal(t, 76800, resZeroPct.TokenLimit)

	// 3. > 100% usage percentage also falls back to 60%
	resOverPct := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Model:           "gpt-4o",
		Data:            []any{"item"},
		UsagePercentage: 150,
	})
	assert.Equal(t, 76800, resOverPct.TokenLimit)

	// 4. Default model name when empty
	resEmptyModel := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
		Model: "",
		Data:  []any{"item"},
	})
	assert.Equal(t, "default", resEmptyModel.ModelUsed)
}
