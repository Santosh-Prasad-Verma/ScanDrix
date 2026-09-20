// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"context"
	"fmt"
	"time"
)

// BatchRunnerConfig controls token budgeting and execution limits.
type BatchRunnerConfig struct {
	DefaultTokenBudget int           `json:"default_token_budget"`
	MaxBatches         int           `json:"max_batches"`
	BatchTimeout       time.Duration `json:"batch_timeout"`
}

// DefaultBatchRunnerConfig returns production default configuration.
func DefaultBatchRunnerConfig() BatchRunnerConfig {
	return BatchRunnerConfig{
		DefaultTokenBudget: 32000,
		MaxBatches:         10,
		BatchTimeout:       3 * time.Minute,
	}
}

// EstimateFileTokens estimates the LLM token consumption of a single changed file.
func EstimateFileTokens(f ChangedFile) int {
	if f.EstimatedTokens > 0 {
		return f.EstimatedTokens
	}

	totalChars := len(f.Patch)
	if totalChars == 0 {
		totalChars = len(f.Content)
	}

	// Average ~3.8 characters per token in code diffs + metadata overhead
	tokens := (totalChars / 4) + 100
	if f.Additions+f.Deletions > 0 {
		tokens += (f.Additions + f.Deletions) * 2
	}
	return tokens
}

// EstimateTotalTokens calculates the total estimated token count across multiple files.
func EstimateTotalTokens(files []ChangedFile) int {
	total := 0
	for _, f := range files {
		total += EstimateFileTokens(f)
	}
	return total
}

// ChunkFilesByTokenBudget packs files into discrete batches where each batch stays within budget.
func ChunkFilesByTokenBudget(files []ChangedFile, tokenBudget int) [][]ChangedFile {
	if len(files) == 0 {
		return nil
	}
	if tokenBudget <= 0 {
		tokenBudget = 32000
	}

	var batches [][]ChangedFile
	var currentBatch []ChangedFile
	currentBatchTokens := 0

	for _, file := range files {
		fileTokens := EstimateFileTokens(file)

		// If a single file exceeds the entire token budget, it gets its own dedicated batch
		if fileTokens >= tokenBudget {
			if len(currentBatch) > 0 {
				batches = append(batches, currentBatch)
				currentBatch = nil
				currentBatchTokens = 0
			}
			batches = append(batches, []ChangedFile{file})
			continue
		}

		// If adding this file would overflow the current batch, seal it and start a new one
		if currentBatchTokens+fileTokens > tokenBudget && len(currentBatch) > 0 {
			batches = append(batches, currentBatch)
			currentBatch = []ChangedFile{file}
			currentBatchTokens = fileTokens
		} else {
			currentBatch = append(currentBatch, file)
			currentBatchTokens += fileTokens
		}
	}

	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// BatchReviewExecutor represents a callable review execution function for a batch of files.
type BatchReviewExecutor func(ctx context.Context, batchInput ReviewAgentInput) (*ReviewAgentOutput, error)

// BatchRunner executes chunked reviews across multiple batches when diffs exceed context windows.
type BatchRunner struct {
	config BatchRunnerConfig
}

// NewBatchRunner constructs a new token-budget batch runner.
func NewBatchRunner(cfg ...BatchRunnerConfig) *BatchRunner {
	c := DefaultBatchRunnerConfig()
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &BatchRunner{config: c}
}

// RunChunkedReview orchestrates chunked execution across all file batches.
func (r *BatchRunner) RunChunkedReview(
	ctx context.Context,
	input ReviewAgentInput,
	executeBatch BatchReviewExecutor,
) (*ReviewAgentOutput, error) {
	startTime := time.Now()

	budget := input.ReviewOptions.MaxTokens
	if budget <= 0 {
		budget = r.config.DefaultTokenBudget
	}

	chunks := ChunkFilesByTokenBudget(input.ChangedFiles, budget)
	if len(chunks) == 0 {
		return &ReviewAgentOutput{
			AgentName:    "batch_runner",
			FinishReason: "completed",
			DurationMs:   time.Since(startTime).Milliseconds(),
		}, nil
	}

	var allFindings []AgentFinding
	var allDiscardedBySeverity []AgentFinding
	var allDiscardedByVerify []AgentFinding
	var allWarnings []ReviewWarning
	totalTurns := 0
	totalTokens := 0
	var batchErrors []error

	// Emit warning if files were split into multiple batches
	if len(chunks) > 1 {
		allWarnings = append(allWarnings, ReviewWarning{
			Kind:                WarningBatchChunked,
			Message:             fmt.Sprintf("PR diff exceeded token budget (%d tokens); review chunked into %d sequential batches", budget, len(chunks)),
			ContextWindowTokens: budget,
			Timestamp:           time.Now().UTC(),
		})
	}

	// Add parent warnings
	allWarnings = append(allWarnings, input.ParentWarnings...)

	maxBatches := r.config.MaxBatches
	if maxBatches <= 0 {
		maxBatches = 10
	}

	for idx, batchFiles := range chunks {
		if idx >= maxBatches {
			allWarnings = append(allWarnings, ReviewWarning{
				Kind:      WarningContextTruncation,
				Message:   fmt.Sprintf("Truncated review after %d batches; remaining %d files were skipped due to batch ceiling", maxBatches, len(chunks)-maxBatches),
				Timestamp: time.Now().UTC(),
			})
			break
		}

		if ctx.Err() != nil {
			allWarnings = append(allWarnings, ReviewWarning{
				Kind:      WarningTimeout,
				Message:   "Context cancelled or timed out during batch review",
				Timestamp: time.Now().UTC(),
			})
			break
		}

		batchInput := input
		batchInput.ChangedFiles = batchFiles
		batchInput.ParentWarnings = allWarnings

		batchCtx, cancel := context.WithTimeout(ctx, r.config.BatchTimeout)
		batchOutput, err := executeBatch(batchCtx, batchInput)
		cancel()

		if err != nil {
			batchErrors = append(batchErrors, err)
			continue
		}

		if batchOutput != nil {
			allFindings = append(allFindings, batchOutput.Findings...)
			allDiscardedBySeverity = append(allDiscardedBySeverity, batchOutput.DiscardedBySeverity...)
			allDiscardedByVerify = append(allDiscardedByVerify, batchOutput.DiscardedByVerify...)
			allWarnings = append(allWarnings, batchOutput.Warnings...)
			totalTurns += batchOutput.TotalTurns
			totalTokens += batchOutput.TokensConsumed
		}
	}

	// If all batches failed with errors and zero findings collected, return error
	if len(batchErrors) > 0 && len(allFindings) == 0 && len(chunks) == len(batchErrors) {
		return nil, fmt.Errorf("all %d batches failed during review execution: %w", len(chunks), batchErrors[0])
	}

	finishReason := "completed"
	if len(chunks) > maxBatches {
		finishReason = "batch-truncated"
	}

	return &ReviewAgentOutput{
		AgentName:           "batch_runner",
		Findings:            allFindings,
		DiscardedBySeverity: allDiscardedBySeverity,
		DiscardedByVerify:   allDiscardedByVerify,
		Warnings:            dedupWarnings(allWarnings),
		TotalTurns:          totalTurns,
		TokensConsumed:      totalTokens,
		DurationMs:          time.Since(startTime).Milliseconds(),
		FinishReason:        finishReason,
	}, nil
}

// dedupWarnings removes duplicate warnings with identical kind and message.
func dedupWarnings(warnings []ReviewWarning) []ReviewWarning {
	if len(warnings) <= 1 {
		return warnings
	}

	seen := make(map[string]struct{})
	var result []ReviewWarning

	for _, w := range warnings {
		key := fmt.Sprintf("%s:%s", w.Kind, w.Message)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, w)
		}
	}
	return result
}
