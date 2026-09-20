package chunking_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/scandrix/backend/internal/core/chunking"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Matrix Tests: Token Estimation on Diverse Types and Edge Cases
// ============================================================================

func TestChunkingMatrix_TokenEstimationTypes(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	t.Run("Nil Item Returns 0", func(t *testing.T) {
		assert.Equal(t, 0, svc.EstimateTokens(nil))
	})

	t.Run("Empty String Returns 0", func(t *testing.T) {
		assert.Equal(t, 0, svc.EstimateTokens(""))
	})

	t.Run("Short String Returns Minimum 1 Token", func(t *testing.T) {
		assert.Equal(t, 1, svc.EstimateTokens("a"))
		assert.Equal(t, 1, svc.EstimateTokens("hi"))
	})

	t.Run("Raw Byte Slice Handling", func(t *testing.T) {
		rawBytes := []byte("func calculateSum(a int, b int) int { return a + b }")
		tokens := svc.EstimateTokens(rawBytes)
		assert.True(t, tokens > 5)
	})

	t.Run("Complex Structured Map and Struct JSON Serialization", func(t *testing.T) {
		type AstNode struct {
			Type     string   `json:"type"`
			Start    int      `json:"start"`
			End      int      `json:"end"`
			Tokens   []string `json:"tokens"`
			Metadata any      `json:"metadata"`
		}

		node := AstNode{
			Type:   "FunctionDeclaration",
			Start:  100,
			End:    250,
			Tokens: []string{"func", "authenticateUser", "(", "req", "*Request", ")", "bool"},
			Metadata: map[string]any{
				"is_exported": true,
				"cyclomatic":  12,
			},
		}

		tokens := svc.EstimateTokens(node)
		assert.True(t, tokens > 20)
	})

	t.Run("Non-Marshallable Object Fallback", func(t *testing.T) {
		// Channel cannot be marshalled into JSON
		ch := make(chan int)
		tokens := svc.EstimateTokens(ch)
		assert.Equal(t, 0, tokens)
	})
}

// ============================================================================
// Matrix Tests: Usage Percentage and Override Headroom Variations
// ============================================================================

func TestChunkingMatrix_UsagePercentageAndHeadroom(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	sampleItems := []any{
		strings.Repeat("a", 700), // ~200 tokens
		strings.Repeat("b", 700), // ~200 tokens
		strings.Repeat("c", 700), // ~200 tokens
		strings.Repeat("d", 700), // ~200 tokens
		strings.Repeat("e", 700), // ~200 tokens
	}

	testCases := []struct {
		name               string
		model              string
		usagePercentage    int
		overrideMaxTokens  int
		expectedTokenLimit int
	}{
		{
			name:               "Claude 3 Sonnet at Standard 60% (200k * 0.6 = 120,000)",
			model:              "claude-3-5-sonnet",
			usagePercentage:    60,
			overrideMaxTokens:  0,
			expectedTokenLimit: 120000,
		},
		{
			name:               "GPT-4o at Conservative 50% (128k * 0.5 = 64,000)",
			model:              "gpt-4o",
			usagePercentage:    50,
			overrideMaxTokens:  0,
			expectedTokenLimit: 64000,
		},
		{
			name:               "Gemini 1.5 Pro at 80% (1M * 0.8 = 800,000)",
			model:              "gemini-1.5-pro",
			usagePercentage:    80,
			overrideMaxTokens:  0,
			expectedTokenLimit: 800000,
		},
		{
			name:               "Invalid Usage Percentage Defaults to 60%",
			model:              "gpt-4o",
			usagePercentage:    -10, // invalid
			overrideMaxTokens:  0,
			expectedTokenLimit: int(128000 * 0.60), // 76800
		},
		{
			name:               "Over 100% Defaults to 60%",
			model:              "gpt-4o",
			usagePercentage:    150, // invalid
			overrideMaxTokens:  0,
			expectedTokenLimit: int(128000 * 0.60), // 76800
		},
		{
			name:               "Explicit Override Max Tokens Takes Precedence",
			model:              "gemini-1.5-pro",
			usagePercentage:    50,
			overrideMaxTokens:  10000,
			expectedTokenLimit: 5000, // 10000 * 0.5
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
				Model:             tc.model,
				Data:              sampleItems,
				UsagePercentage:   tc.usagePercentage,
				OverrideMaxTokens: tc.overrideMaxTokens,
			})

			assert.Equal(t, tc.expectedTokenLimit, res.TokenLimit)
			assert.Equal(t, len(sampleItems), res.TotalItems)
			assert.Equal(t, tc.model, res.ModelUsed)
		})
	}
}

// ============================================================================
// Matrix Tests: Boundary Packing & Exact Limit Chunk Splitting
// ============================================================================

func TestChunkingMatrix_ExactLimitBoundarySplitting(t *testing.T) {
	svc := chunking.NewTokenChunkingService()

	// Create items with exact token sizes
	// 35 chars = 10 tokens
	item10Tokens := strings.Repeat("x", 35)

	t.Run("Cumulative Items Exactly Equal Limit", func(t *testing.T) {
		// Target limit = 1000 tokens (minimum limit fallback when window small)
		// 100 items of 10 tokens = exactly 1000 tokens
		data := make([]any, 100)
		for i := 0; i < 100; i++ {
			data[i] = item10Tokens
		}

		res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
			Data:              data,
			OverrideMaxTokens: 2000,
			UsagePercentage:   50, // tokenLimit = 1000
		})

		assert.Equal(t, 1000, res.TokenLimit)
		assert.Equal(t, 100, res.TotalItems)
		// Exactly fits into 1 chunk
		assert.Equal(t, 1, res.TotalChunks)
		assert.Len(t, res.Chunks, 1)
		assert.Equal(t, 1000, res.TokensPerChunk[0])
	})

	t.Run("One Additional Item Triggers New Chunk", func(t *testing.T) {
		// 101 items of 10 tokens = 1010 tokens -> must spill into 2nd chunk
		data := make([]any, 101)
		for i := 0; i < 101; i++ {
			data[i] = item10Tokens
		}

		res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
			Data:              data,
			OverrideMaxTokens: 2000,
			UsagePercentage:   50, // tokenLimit = 1000
		})

		assert.Equal(t, 1000, res.TokenLimit)
		assert.Equal(t, 2, res.TotalChunks)
		assert.Len(t, res.Chunks, 2)
		assert.Equal(t, 1000, res.TokensPerChunk[0])
		assert.Equal(t, 10, res.TokensPerChunk[1])
		assert.Len(t, res.Chunks[0], 100)
		assert.Len(t, res.Chunks[1], 1)
	})

	t.Run("Oversized Item Larger Than Token Limit Isolated", func(t *testing.T) {
		// Item of 1500 tokens (exceeds 1000 token limit)
		// 1500 * 3.5 = 5250 chars
		oversizedItem := strings.Repeat("z", 5250)

		data := []any{
			item10Tokens,  // chunk 1
			oversizedItem, // chunk 2 (isolated)
			item10Tokens,  // chunk 3
		}

		res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
			Data:              data,
			OverrideMaxTokens: 2000,
			UsagePercentage:   50, // tokenLimit = 1000
		})

		assert.Equal(t, 3, res.TotalChunks)
		assert.Len(t, res.Chunks, 3)

		// Chunk 0: first normal item
		assert.Len(t, res.Chunks[0], 1)
		assert.Equal(t, item10Tokens, res.Chunks[0][0])

		// Chunk 1: oversized isolated item
		assert.Len(t, res.Chunks[1], 1)
		assert.Equal(t, oversizedItem, res.Chunks[1][0])
		assert.True(t, res.TokensPerChunk[1] >= 1500)

		// Chunk 2: remaining normal item
		assert.Len(t, res.Chunks[2], 1)
		assert.Equal(t, item10Tokens, res.Chunks[2][0])
	})
}

// ============================================================================
// Matrix Tests: High-Concurrency AST Chunking Stress
// ============================================================================

func TestChunkingMatrix_HighConcurrencyASTChunkingStress(t *testing.T) {
	svc := chunking.NewTokenChunkingService()
	const numGoroutines = 40
	const iterationsPerRoutine = 25

	// Pre-generate 50 mock AST diff items
	mockASTItems := make([]any, 50)
	for i := 0; i < 50; i++ {
		mockASTItems[i] = map[string]any{
			"file": fmt.Sprintf("internal/review/checker_%d.go", i),
			"diff": fmt.Sprintf("@@ -10,%d +10,%d @@ func Validate_%d() { return true }", i+5, i+15, i),
			"ast": map[string]any{
				"kind":     "FunctionDecl",
				"exported": true,
				"loc":      map[string]int{"line": i * 10, "col": 1},
			},
		}
	}

	var totalProcessed atomic.Int64
	var totalChunksCreated atomic.Int64

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	models := []string{
		"gemini-1.5-pro",
		"claude-3-5-sonnet",
		"gpt-4o",
		"gpt-4",
		"custom-local",
	}

	for g := 0; g < numGoroutines; g++ {
		go func(routineID int) {
			defer wg.Done()
			for iter := 0; iter < iterationsPerRoutine; iter++ {
				model := models[(routineID+iter)%len(models)]

				res := svc.ChunkDataByTokens(chunking.TokenChunkingOptions{
					Model:           model,
					Data:            mockASTItems,
					UsagePercentage: 60,
				})

				require.NotEmpty(t, res.Chunks)
				assert.Equal(t, len(mockASTItems), res.TotalItems)
				assert.Equal(t, len(res.Chunks), res.TotalChunks)
				assert.Equal(t, len(res.Chunks), len(res.TokensPerChunk))

				totalProcessed.Add(int64(res.TotalItems))
				totalChunksCreated.Add(int64(res.TotalChunks))
			}
		}(g)
	}

	wg.Wait()

	expectedItemsProcessed := int64(numGoroutines * iterationsPerRoutine * len(mockASTItems))
	assert.Equal(t, expectedItemsProcessed, totalProcessed.Load())
	assert.True(t, totalChunksCreated.Load() > 0)
}
