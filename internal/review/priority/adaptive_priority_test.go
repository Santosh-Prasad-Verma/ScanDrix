// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package priority

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveProfile_ClassificationBands(t *testing.T) {
	testCases := []struct {
		name                 string
		tokens               int
		expectedKind         AdaptiveProfileKind
		expectedCompact      bool
		expectedDropGraph    bool
		expectedAllOptional  bool
		expectedMaxChars     bool
		expectedSkipHeavy    bool
		expectedUncondFilter bool
	}{
		{
			name:                 "Full Fidelity at 128K tokens",
			tokens:               128000,
			expectedKind:         ProfileFull,
			expectedCompact:      false,
			expectedDropGraph:    false,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    false,
			expectedUncondFilter: false,
		},
		{
			name:                 "Light Fidelity at 48K tokens",
			tokens:               48000,
			expectedKind:         ProfileLight,
			expectedCompact:      false,
			expectedDropGraph:    true,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    true,
			expectedUncondFilter: false,
		},
		{
			name:                 "Compact Fidelity at 24K tokens",
			tokens:               24000,
			expectedKind:         ProfileCompact,
			expectedCompact:      true,
			expectedDropGraph:    true,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    true,
			expectedUncondFilter: true,
		},
		{
			name:                 "Minimal Fidelity at 10K tokens",
			tokens:               10000,
			expectedKind:         ProfileMinimal,
			expectedCompact:      true,
			expectedDropGraph:    true,
			expectedAllOptional:  true,
			expectedMaxChars:     true,
			expectedSkipHeavy:    true,
			expectedUncondFilter: true,
		},
		{
			name:                 "Unviable at 4K tokens",
			tokens:               4000,
			expectedKind:         ProfileUnviable,
			expectedCompact:      false,
			expectedDropGraph:    false,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    false,
			expectedUncondFilter: false,
		},
		{
			name:                 "Unviable at 0 tokens",
			tokens:               0,
			expectedKind:         ProfileUnviable,
			expectedCompact:      false,
			expectedDropGraph:    false,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    false,
			expectedUncondFilter: false,
		},
		{
			name:                 "Unviable at negative tokens",
			tokens:               -100,
			expectedKind:         ProfileUnviable,
			expectedCompact:      false,
			expectedDropGraph:    false,
			expectedAllOptional:  false,
			expectedMaxChars:     false,
			expectedSkipHeavy:    false,
			expectedUncondFilter: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := ResolveAdaptiveProfile(tc.tokens)
			assert.Equal(t, tc.expectedKind, p.Kind)
			assert.Equal(t, tc.expectedCompact, p.CompactPrompt)
			assert.Equal(t, tc.expectedDropGraph, p.DropCallGraph)
			assert.Equal(t, tc.expectedAllOptional, p.AllOptional)
			assert.Equal(t, tc.expectedSkipHeavy, p.SkipHeavyPasses)
			assert.Equal(t, tc.expectedUncondFilter, p.LowSignalFilterUnconditional)

			if tc.expectedMaxChars {
				require.NotNil(t, p.MaxDiffChars)
				assert.Equal(t, MinimalProfileMaxDiffChars, *p.MaxDiffChars)
			} else {
				assert.Nil(t, p.MaxDiffChars)
			}
		})
	}
}

func TestContextFitPlanner_OverheadAndBudget(t *testing.T) {
	t.Run("EstimateNonDiffOverheadTokens", func(t *testing.T) {
		standardOverhead := EstimateNonDiffOverheadTokens(false)
		assert.Equal(t, 62000/CharsPerToken, standardOverhead)

		compactOverhead := EstimateNonDiffOverheadTokens(true)
		assert.Equal(t, 48000/CharsPerToken, compactOverhead)
		assert.Less(t, compactOverhead, standardOverhead)
	})

	t.Run("AssertContextWindowFitsOverhead", func(t *testing.T) {
		// 128K context window easily fits overhead
		err := AssertContextWindowFitsOverhead(128000, false)
		assert.NoError(t, err)

		// 32K context window: 32000 * 0.55 = 17600 max allowed overhead.
		// Standard overhead is 15500 tokens -> fits!
		err = AssertContextWindowFitsOverhead(32000, false)
		assert.NoError(t, err)

		// 16K context window: 16000 * 0.55 = 8800 tokens max allowed overhead.
		// Standard overhead is 15500 -> too large!
		err = AssertContextWindowFitsOverhead(16000, false)
		require.Error(t, err)
		var tooSmallErr *ContextWindowTooSmallError
		assert.ErrorAs(t, err, &tooSmallErr)
		assert.Equal(t, 16000, tooSmallErr.ContextWindowTokens)
	})
}

func TestContextFitPlanner_AggressiveFilter(t *testing.T) {
	files := []ChangedFileReviewItem{
		{Filename: "internal/service/order.go", Patch: "+ code"},
		{Filename: "internal/service/order.test.ts", Patch: "+ test"},
		{Filename: "internal/service/order.spec.go", Patch: "+ spec"},
		{Filename: "web/styles/theme.css", Patch: "+ css"},
		{Filename: "docs/readme.md", Patch: "+ doc"},
		{Filename: "cmd/api/main.go", Patch: "+ main"},
	}

	// 1. Unconditional filter (compact profile)
	filtered := ApplyLargePrAggressiveFilter(files, true)
	require.Len(t, filtered, 2)
	assert.Equal(t, "internal/service/order.go", filtered[0].Filename)
	assert.Equal(t, "cmd/api/main.go", filtered[1].Filename)

	// 2. Small PR without unconditional flag does not filter
	unfiltered := ApplyLargePrAggressiveFilter(files, false)
	assert.Len(t, unfiltered, len(files))

	// 3. Fallback when all files would be filtered
	allTests := []ChangedFileReviewItem{
		{Filename: "pkg/calc_test.go", Patch: "+ test"},
	}
	res := ApplyLargePrAggressiveFilter(allTests, true)
	assert.Len(t, res, 1)
}

func TestContextFitPlanner_ChunkFilesByTokenBudget(t *testing.T) {
	files := []ChangedFileReviewItem{
		{Filename: "file1.go", Patch: strings.Repeat("x", 400)}, // ~100 tokens
		{Filename: "file2.go", Patch: strings.Repeat("y", 800)}, // ~200 tokens
		{Filename: "file3.go", Patch: strings.Repeat("z", 600)}, // ~150 tokens
		{Filename: "file4.go", Patch: strings.Repeat("w", 2000)}, // ~500 tokens (oversized)
		{Filename: "file5.go", Patch: strings.Repeat("a", 400)}, // ~100 tokens
	}

	// Budget of 350 tokens per chunk
	chunks := ChunkFilesByTokenBudget(files, 350)
	require.NotEmpty(t, chunks)

	totalFilesAcrossChunks := 0
	for _, c := range chunks {
		totalFilesAcrossChunks += len(c)
	}
	assert.Equal(t, len(files), totalFilesAcrossChunks)
}

func TestReviewWarnings_LifecycleAndDedup(t *testing.T) {
	w1 := ReviewWarning{
		Kind:                WarningPromptCompacted,
		Reason:              ReasonSmallContextWindow,
		ContextWindowTokens: 16000,
		ModelName:           "claude-3-haiku",
		Detail:              "workflow rules trimmed",
		AgentName:           "bug",
	}

	w2 := ReviewWarning{
		Kind:                WarningPromptCompacted,
		Reason:              ReasonSmallContextWindow,
		ContextWindowTokens: 16000,
		ModelName:           "claude-3-haiku",
		Detail:              "output format compressed",
		AgentName:           "security",
	}

	w3 := BuildProviderFallbackWarning("gpt-4o", "claude-3-5-sonnet", "performance")

	warnings := []ReviewWarning{w1, w2, w3}
	deduped := DedupReviewWarnings(warnings)

	// w1 and w2 fold into one PROMPT_COMPACTED notice with concatenated details and cleared AgentName
	require.Len(t, deduped, 2)

	assert.Equal(t, WarningPromptCompacted, deduped[0].Kind)
	assert.Equal(t, "", deduped[0].AgentName)
	assert.Contains(t, deduped[0].Detail, "workflow rules trimmed")
	assert.Contains(t, deduped[0].Detail, "output format compressed")

	assert.Equal(t, WarningProviderFallback, deduped[1].Kind)
	assert.Equal(t, "performance", deduped[1].AgentName)

	// Markdown generation
	md := FormatWarningsMarkdown(deduped)
	assert.Contains(t, md, "<details>")
	assert.Contains(t, md, "Pipeline Fidelity Notices (2)")
	assert.Contains(t, md, "Prompt Compacted")
	assert.Contains(t, md, "Model Provider Failover")
}

func TestReviewWarnings_ConcurrentStress(t *testing.T) {
	workers := 20
	ops := 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for w := 0; w < workers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				warnings := []ReviewWarning{
					{
						Kind:                WarningPromptCompacted,
						Reason:              ReasonSmallContextWindow,
						ContextWindowTokens: 16000,
						ModelName:           fmt.Sprintf("model-%d", i%3),
						Detail:              fmt.Sprintf("worker-%d-iter-%d", workerID, i),
						AgentName:           fmt.Sprintf("agent-%d", workerID),
					},
					BuildProviderFallbackWarning("m1", "m2", "agent"),
				}

				deduped := DedupReviewWarnings(warnings)
				_ = FormatWarningsMarkdown(deduped)
			}
		}(w)
	}

	wg.Wait()
}
