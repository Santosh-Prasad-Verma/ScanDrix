// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkFilesByTokenBudget(t *testing.T) {
	// Create 4 files with known sizes
	f1 := ChangedFile{Filename: "file1.go", Patch: strings.Repeat("a", 4000)} // ~1100 tokens
	f2 := ChangedFile{Filename: "file2.go", Patch: strings.Repeat("b", 4000)} // ~1100 tokens
	f3 := ChangedFile{Filename: "file3.go", Patch: strings.Repeat("c", 4000)} // ~1100 tokens
	f4 := ChangedFile{Filename: "file4.go", Patch: strings.Repeat("d", 4000)} // ~1100 tokens

	files := []ChangedFile{f1, f2, f3, f4}

	// Case 1: Large budget fits all in 1 chunk
	chunks := ChunkFilesByTokenBudget(files, 10000)
	require.Len(t, chunks, 1)
	assert.Len(t, chunks[0], 4)

	// Case 2: Budget of 2000 tokens splits into multiple chunks (2 files each)
	chunks = ChunkFilesByTokenBudget(files, 2400)
	require.Len(t, chunks, 2)
	assert.Len(t, chunks[0], 2)
	assert.Len(t, chunks[1], 2)

	// Case 3: Very small budget forces 1 file per chunk
	chunks = ChunkFilesByTokenBudget(files, 500)
	require.Len(t, chunks, 4)
}

func TestBatchRunner_RunChunkedReview(t *testing.T) {
	runner := NewBatchRunner()

	files := []ChangedFile{
		{Filename: "f1.go", Patch: strings.Repeat("x", 2000)},
		{Filename: "f2.go", Patch: strings.Repeat("y", 2000)},
	}

	input := ReviewAgentInput{
		ChangedFiles: files,
		ReviewOptions: ReviewOptions{
			MaxTokens: 1000, // force chunking
		},
	}

	callCount := 0
	executor := func(ctx context.Context, batchInput ReviewAgentInput) (*ReviewAgentOutput, error) {
		callCount++
		return &ReviewAgentOutput{
			AgentName: "mock_specialist",
			Findings: []AgentFinding{
				{
					ID:       uuid.New(),
					FilePath: batchInput.ChangedFiles[0].Filename,
					Title:    "Test finding",
					Severity: models.SeverityMedium,
				},
			},
			TotalTurns:     2,
			TokensConsumed: 500,
		}, nil
	}

	output, err := runner.RunChunkedReview(context.Background(), input, executor)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should have executed 2 batches
	assert.Equal(t, 2, callCount)
	assert.Len(t, output.Findings, 2)
	assert.Equal(t, 4, output.TotalTurns)
	assert.Equal(t, 1000, output.TokensConsumed)
	assert.True(t, len(output.Warnings) >= 1)
	assert.Equal(t, WarningBatchChunked, output.Warnings[0].Kind)
}
