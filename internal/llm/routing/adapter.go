// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package routing

import (
	"github.com/scandrix/backend/internal/llm/byok"
)

type staticResolverBridge struct {
	strategy *StaticTaskStrategy
}

func (b *staticResolverBridge) Resolve(task byok.LlmTask, ctxOverrideID, ctxOverrideName string, config *byok.BYOKConfig) (modelID string, usedFallback bool, reason string) {
	ctx := RequestContext{
		OverrideModelID:   ctxOverrideID,
		OverrideModelName: ctxOverrideName,
	}
	verdict := b.strategy.Resolve(task, ctx, config)
	return verdict.ModelID, verdict.UsedFallback, verdict.Reason
}

func (b *staticResolverBridge) ResolveFallback(task byok.LlmTask, config *byok.BYOKConfig) (modelID string, ok bool) {
	verdict := b.strategy.ResolveFallback(task, config)
	return verdict.ModelID, verdict.ModelID != ""
}

func init() {
	byok.RegisterTaskResolver(&staticResolverBridge{
		strategy: NewStaticTaskStrategy(),
	})
}
