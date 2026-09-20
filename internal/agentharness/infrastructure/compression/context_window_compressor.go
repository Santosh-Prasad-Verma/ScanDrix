// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package compression

import (
	"math"
	"os"
	"strconv"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// DefaultSafetyMarginRatio is the fraction of context kept free for provider response and drift.
const DefaultSafetyMarginRatio = 0.08

// ContextWindowCompressorOptions configures the 2-tier context compressor.
type ContextWindowCompressorOptions struct {
	OverheadTokens     int
	SafetyMarginTokens int
	SafetyMarginRatio  float64
	Config             *CompressionConfig
}

// ContextWindowCompressor implements contracts.Compressor.
// It applies a 2-tier compaction strategy:
// 1. Soft pass: truncates older tool results when usage crosses the trigger ratio.
// 2. Hard clamp: guarantees the message history provably fits within the real token budget.
type ContextWindowCompressor struct {
	contextWindowTokens int
	overheadTokens      int
	safetyMarginTokens  int
	config              CompressionConfig
}

// NewContextWindowCompressor constructs a ContextWindowCompressor.
func NewContextWindowCompressor(contextWindowTokens int, opts ...ContextWindowCompressorOptions) *ContextWindowCompressor {
	var opt ContextWindowCompressorOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	ratio := opt.SafetyMarginRatio
	if ratio <= 0 {
		if v := os.Getenv("COMPRESSION_SAFETY_MARGIN_RATIO"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
				ratio = f
			}
		}
		if ratio <= 0 {
			ratio = DefaultSafetyMarginRatio
		}
	}

	safetyMargin := opt.SafetyMarginTokens
	if safetyMargin <= 0 && contextWindowTokens > 0 {
		safetyMargin = int(math.Ceil(float64(contextWindowTokens) * ratio))
	}

	cfg := DefaultCompressionConfig()
	if opt.Config != nil {
		cfg = *opt.Config
	}

	return &ContextWindowCompressor{
		contextWindowTokens: contextWindowTokens,
		overheadTokens:      opt.OverheadTokens,
		safetyMarginTokens:  safetyMargin,
		config:              cfg,
	}
}

// MaybeCompress evaluates the message window and returns a compressed window if necessary.
func (c *ContextWindowCompressor) MaybeCompress(messages []contracts.AgentMessage) *contracts.CompressionResult {
	if c.contextWindowTokens <= 0 || len(messages) == 0 {
		return nil
	}

	budget := c.contextWindowTokens - c.overheadTokens - c.safetyMarginTokens
	if budget < 0 {
		budget = 0
	}

	current := EstimateMessagesTokens(messages)
	usage := current + c.overheadTokens

	softTrigger := float64(usage) > float64(c.contextWindowTokens)*c.config.CompressionThresholdRatio
	overBudget := current > budget

	if !softTrigger && !overBudget {
		return nil
	}

	// 1. Soft pass: truncate older tool results
	compressed := CompressMessages(messages, nil, c.config)
	after := EstimateMessagesTokens(compressed)

	// 2. Hard clamp: ensure messages provably fit budget
	if after > budget {
		compressed = ClampMessagesToBudget(compressed, budget, c.config)
		after = EstimateMessagesTokens(compressed)
	}

	// If no savings achieved and wasn't over budget, leave unchanged
	if after >= current && !overBudget {
		return nil
	}

	return &contracts.CompressionResult{
		Messages:     compressed,
		BeforeTokens: current,
		AfterTokens:  after,
	}
}
