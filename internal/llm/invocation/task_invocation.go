// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package invocation

import (
	"github.com/scandrix/backend/internal/llm/byok"
)

// TaskInvocation represents the complete resolved invocation context for a specific task.
// Composes routing decision + model access + tuning + reasoning + usage identity.
type TaskInvocation struct {
	ModelInvocation
	Slot          *byok.NormalizedModel
	VerdictReason string
	UsedFallback  bool
	UsageIdentity ModelIdentity
}

// ResolveTaskInvocationOptions specifies parameters for task-level model resolution.
type ResolveTaskInvocationOptions struct {
	ResolveModelInvocationOptions
	OverrideModelID   string
	OverrideModelName string
}

// ResolveTaskInvocation resolves a task over an organization's BYOK config into a ready-to-call invocation.
// The single assembly point ensuring that routing, model tuning, reasoning, and usage tracking never drift.
func ResolveTaskInvocation(
	config *byok.BYOKConfig,
	task byok.LlmTask,
	opts ResolveTaskInvocationOptions,
) TaskInvocation {
	slot, usedFallback, reason := byok.ResolveTaskSlot(config, task, opts.OverrideModelID, opts.OverrideModelName)
	invocation := ResolveModelConfig(slot, opts.ResolveModelInvocationOptions)

	usageId := AgentModelIdentity(slot)
	usageId.Model = invocation.ModelName

	return TaskInvocation{
		ModelInvocation: invocation,
		Slot:            slot,
		VerdictReason:   reason,
		UsedFallback:    usedFallback,
		UsageIdentity:   usageId,
	}
}
