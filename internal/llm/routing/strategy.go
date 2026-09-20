// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package routing

import (
	"github.com/scandrix/backend/internal/llm/byok"
)

// RequestContext provides per-request routing parameters (e.g. folder/repo model overrides).
type RequestContext struct {
	OverrideModelID   string
	OverrideModelName string
}

// RoutingVerdict represents the decision produced by a RoutingStrategy.
type RoutingVerdict struct {
	ModelID      string
	ModelName    string
	Reason       string
	UsedFallback bool
}

// RoutingStrategy abstracts task-to-model resolution.
type RoutingStrategy interface {
	Resolve(task byok.LlmTask, ctx RequestContext, config *byok.BYOKConfig) RoutingVerdict
	ResolveFallback(task byok.LlmTask, config *byok.BYOKConfig) RoutingVerdict
}
