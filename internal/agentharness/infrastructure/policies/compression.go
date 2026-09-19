// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package policies

import (
	"context"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

// CompressionPolicy intercepts the message window during PrepareStep and delegates
// compaction to the injected Compressor implementation when context threshold triggers.
type CompressionPolicy struct {
	contracts.BasePolicy
	compressor contracts.Compressor
}

// NewCompressionPolicy constructs a CompressionPolicy with an injected Compressor port.
func NewCompressionPolicy(compressor contracts.Compressor) *CompressionPolicy {
	return &CompressionPolicy{
		BasePolicy: contracts.BasePolicy{PolicyName: "compression"},
		compressor: compressor,
	}
}

func (p *CompressionPolicy) PrepareStep(ctx context.Context, view contracts.StepView) (contracts.StepDirectives, error) {
	if p.compressor == nil {
		return contracts.StepDirectives{}, nil
	}

	result := p.compressor.MaybeCompress(view.Messages)
	if result == nil {
		return contracts.StepDirectives{}, nil
	}

	return contracts.StepDirectives{
		Messages: result.Messages,
		Emit: []contracts.TraceEvent{
			{
				At:     time.Now(),
				Source: p.Name(),
				Kind:   "context.compress",
				Detail: map[string]any{
					"beforeTokens":   result.BeforeTokens,
					"afterTokens":    result.AfterTokens,
					"savedTokens":    result.BeforeTokens - result.AfterTokens,
					"beforeMessages": len(view.Messages),
					"afterMessages":  len(result.Messages),
				},
			},
		},
	}, nil
}
