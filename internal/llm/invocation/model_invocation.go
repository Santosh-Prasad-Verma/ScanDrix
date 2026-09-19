// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package invocation

import (
	"fmt"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/reasoning"
	"github.com/scandrix/backend/internal/llm/sampling"
)

// ModelInvocation encapsulates the model configuration, tuning, and provider-specific options for an LLM call.
type ModelInvocation struct {
	Slot            *byok.NormalizedModel
	ModelName       string
	CallOptions     sampling.SlotCallOptions
	ProviderOptions map[string]any
}

// ResolveModelInvocationOptions defines options provided to resolve model invocation parameters.
type ResolveModelInvocationOptions struct {
	RunName                  string
	SuppressReasoning        bool
	ReasoningEffortDefault   string // Defaults to "low" if unset
	OpenRouterProviderOrder  []string
	OpenRouterAllowFallbacks *bool
	DefaultModelOverride     string
}

// ResolveModelConfig turns a resolved slot (or nil for env/managed default) into a ready-to-call invocation.
// Composes sampling options, reasoning effort/options, and provider options into a unified structure.
func ResolveModelConfig(slot *byok.NormalizedModel, opts ResolveModelInvocationOptions) ModelInvocation {
	fallbackEffort := opts.ReasoningEffortDefault
	if fallbackEffort == "" {
		fallbackEffort = "low"
	}

	modelName := opts.DefaultModelOverride
	if slot != nil {
		if slot.Provider != "" {
			modelName = fmt.Sprintf("%s:%s", slot.Provider, slot.Model)
		} else {
			modelName = slot.Model
		}
	} else if modelName == "" {
		modelName = "system:default"
	}

	callOpts := sampling.ResolveSlotCallOptions(slot)

	var providerStr string
	var slotModel string
	var reasoningEffort string
	var reasoningOverride string
	var openrouterOrders []string
	var openrouterFallbacks *bool

	if slot != nil {
		providerStr = string(slot.Provider)
		slotModel = slot.Model
		openrouterOrders = slot.OpenRouterProviderOrder
		openrouterFallbacks = slot.OpenRouterAllowFallback
		reasoningOverride = slot.ReasoningConfigOverride

		if opts.SuppressReasoning {
			reasoningEffort = "none"
			reasoningOverride = ""
		} else if slot.ReasoningEffort != "" {
			reasoningEffort = slot.ReasoningEffort
		} else if def := reasoning.DefaultReasoningEffortFor(slot); def != "" {
			reasoningEffort = def
		} else {
			reasoningEffort = fallbackEffort
		}
	} else {
		if opts.SuppressReasoning {
			reasoningEffort = "none"
		} else {
			reasoningEffort = fallbackEffort
		}
	}

	if len(opts.OpenRouterProviderOrder) > 0 {
		openrouterOrders = opts.OpenRouterProviderOrder
	}
	if opts.OpenRouterAllowFallbacks != nil {
		openrouterFallbacks = opts.OpenRouterAllowFallbacks
	}

	provOpts := reasoning.BuildProviderOptions(reasoning.ProviderOptionsInput{
		RunName:                  opts.RunName,
		ReasoningEffort:          reasoningEffort,
		ReasoningConfigOverride:  reasoningOverride,
		Provider:                 providerStr,
		ModelName:                slotModel,
		OpenRouterProviderOrder:  openrouterOrders,
		OpenRouterAllowFallbacks: openrouterFallbacks,
	})

	return ModelInvocation{
		Slot:            slot,
		ModelName:       modelName,
		CallOptions:     callOpts,
		ProviderOptions: provOpts,
	}
}
