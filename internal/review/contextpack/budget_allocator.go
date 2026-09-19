// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package contextpack

import (
	"math"
	"sort"
)

const (
	CodeCharsPerToken  = 2.8
	ProseCharsPerToken = 3.8
)

// EstimateTokens calculates the token size of text based on whether it is code or prose.
func EstimateTokens(text string, isCode bool) int {
	if text == "" {
		return 0
	}
	ratio := ProseCharsPerToken
	if isCode {
		ratio = CodeCharsPerToken
	}
	return int(math.Ceil(float64(len(text)) / ratio))
}

// BudgetAllocator enforces per-layer and total token caps with priority-based compaction.
type BudgetAllocator struct{}

// NewBudgetAllocator constructs a budget allocator.
func NewBudgetAllocator() *BudgetAllocator {
	return &BudgetAllocator{}
}

// Allocate enforces token budgets across layers, prioritizing higher-priority layers first.
func (a *BudgetAllocator) Allocate(layers []ContextLayer, config ContextBudgetConfig) ([]ContextLayer, int) {
	if len(layers) == 0 {
		return nil, 0
	}

	// 1. First pass: Apply individual per-layer budget caps
	processed := make([]ContextLayer, len(layers))
	for i, l := range layers {
		layerCopy := l
		isCode := l.Kind == LayerDiff || l.Kind == LayerKnowledgeGraph

		if layerCopy.EstimatedTokens <= 0 {
			layerCopy.EstimatedTokens = EstimateTokens(layerCopy.Content, isCode)
		}

		capBudget, hasCap := config.LayerBudgets[l.Kind]
		if hasCap && capBudget > 0 && layerCopy.EstimatedTokens > capBudget {
			// Truncate content to fit layer budget
			ratio := ProseCharsPerToken
			if isCode {
				ratio = CodeCharsPerToken
			}
			maxChars := int(float64(capBudget) * ratio)
			if maxChars < len(layerCopy.Content) {
				layerCopy.Content = layerCopy.Content[:maxChars] + "\n\n... [Layer truncated to meet budget cap] ...\n"
				layerCopy.EstimatedTokens = capBudget
				layerCopy.Truncated = true
			}
		}

		processed[i] = layerCopy
	}

	// 2. Second pass: If total budget is exceeded, drop layers by priority ascending
	totalTokens := 0
	for _, l := range processed {
		totalTokens += l.EstimatedTokens
	}

	if config.TotalBudgetTokens <= 0 || totalTokens <= config.TotalBudgetTokens {
		return processed, totalTokens
	}

	// Sort indices by priority ascending (lowest priority dropped first)
	type layerIndex struct {
		index    int
		priority int
	}
	indices := make([]layerIndex, len(processed))
	for i, l := range processed {
		indices[i] = layerIndex{index: i, priority: l.Priority}
	}
	sort.Slice(indices, func(i, j int) bool {
		return indices[i].priority < indices[j].priority
	})

	for _, item := range indices {
		idx := item.index
		if totalTokens <= config.TotalBudgetTokens {
			break
		}

		// Don't drop LayerDiff if it's the primary core layer unless absolutely unavoidable
		if processed[idx].Kind == LayerDiff && len(processed) > 1 {
			continue
		}

		totalTokens -= processed[idx].EstimatedTokens
		processed[idx].DroppedForBudget = true
		processed[idx].Content = ""
		processed[idx].EstimatedTokens = 0
	}

	// Recalculate true total
	finalTotal := 0
	for _, l := range processed {
		if !l.DroppedForBudget {
			finalTotal += l.EstimatedTokens
		}
	}

	return processed, finalTotal
}
