// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

// TaskResolver defines the contract for task-to-slot routing.
type TaskResolver interface {
	Resolve(task LlmTask, ctxOverrideID, ctxOverrideName string, config *BYOKConfig) (modelID string, usedFallback bool, reason string)
	ResolveFallback(task LlmTask, config *BYOKConfig) (modelID string, ok bool)
}

var globalTaskResolver TaskResolver

// RegisterTaskResolver registers the concrete routing strategy.
func RegisterTaskResolver(r TaskResolver) {
	globalTaskResolver = r
}

// ResolveTaskSlot resolves the active slot and attaches its runtime fallback.
func ResolveTaskSlot(config *BYOKConfig, task LlmTask, overrideID, overrideName string) (slot *NormalizedModel, usedFallback bool, reason string) {
	if config == nil {
		return nil, false, "no byok config provided"
	}

	if globalTaskResolver == nil {
		// Fallback to default slot resolution if resolver is not registered
		defaultSlot := ResolveDefaultSlot(config)
		return defaultSlot, false, "default slot"
	}

	winningID, usedFB, res := globalTaskResolver.Resolve(task, overrideID, overrideName, config)
	if winningID == "" {
		return nil, false, res
	}

	primarySlot := ResolveModelSlot(config, winningID)
	if primarySlot == nil {
		return nil, false, "winning model could not be materialized"
	}

	primarySlot.Route = task
	primarySlot.UsedFallback = usedFB

	// Attach fallback if configured and distinct
	if !usedFB && config.Routing.FallbackModelID != "" && config.Routing.FallbackModelID != winningID {
		if fbID, ok := globalTaskResolver.ResolveFallback(task, config); ok && fbID != "" && fbID != winningID {
			if fbSlot := ResolveModelSlot(config, fbID); fbSlot != nil {
				fbSlot.Route = task
				fbSlot.UsedFallback = true
				primarySlot.Fallback = fbSlot.ToFallbackSlot()
			}
		}
	}

	return primarySlot, usedFB, res
}

// IsBYOKSlotConfigured determines whether a slot carries usable credentials to run inference.
func IsBYOKSlotConfigured(slot *NormalizedModel) bool {
	if slot == nil {
		return false
	}
	if slot.Provider == ProviderAmazonBedrock {
		return slot.AWSBearerToken != "" || (slot.AWSAccessKeyID != "" && slot.AWSSecretAccessKey != "")
	}
	return slot.APIKey != ""
}
