// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package routing

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// TaskRoutingFallback defines task inheritance when a task has no explicit override.
var TaskRoutingFallback = map[byok.LlmTask]byok.LlmTask{}

type capabilityRequirement struct {
	name      string
	satisfied func(caps kernel.ModelCapabilities) bool
}

var structuredOutputRequirement = capabilityRequirement{
	name: "structuredOutput",
	satisfied: func(c kernel.ModelCapabilities) bool {
		return c.StructuredOutput != "none" || c.ToolCalling == "native"
	},
}

var toolCallingRequirement = capabilityRequirement{
	name: "toolCalling",
	satisfied: func(c kernel.ModelCapabilities) bool {
		return c.ToolCalling == "native"
	},
}

var taskCapabilityRequirements = map[byok.LlmTask]*capabilityRequirement{
	byok.TaskCodeReview:         &structuredOutputRequirement,
	byok.TaskDrixyRulesReview:   &structuredOutputRequirement,
	byok.TaskRuleGeneration:     &structuredOutputRequirement,
	byok.TaskBusinessValidation: &toolCallingRequirement,
	byok.TaskPRSummary:          nil, // Any model qualifies
	byok.TaskConversation:       &toolCallingRequirement,
}

// StaticTaskStrategy implements manual priority-based routing:
// Context Override > Task Override > Inherited Task Override > Default Model > (Gate Failure) Fallback Model.
type StaticTaskStrategy struct{}

// NewStaticTaskStrategy initializes a new manual static task routing strategy.
func NewStaticTaskStrategy() *StaticTaskStrategy {
	return &StaticTaskStrategy{}
}

type candidateModel struct {
	model        byok.BYOKModelConfig
	tier         string
	nameOverride string
	isFallback   bool
}

// Resolve identifies the winning model configuration for the requested task.
func (s *StaticTaskStrategy) Resolve(task byok.LlmTask, ctx RequestContext, config *byok.BYOKConfig) RoutingVerdict {
	if config == nil || len(config.Models) == 0 {
		return RoutingVerdict{Reason: "no models configured in workspace"}
	}

	modelMap := make(map[string]byok.BYOKModelConfig)
	for _, m := range config.Models {
		modelMap[m.ID] = m
	}

	credMap := make(map[string]byok.BYOKCredential)
	for _, c := range config.Credentials {
		credMap[c.ID] = c
	}

	// Identify base routed model (task override or default) for legacy name overrides
	var baseRoutedModel *byok.BYOKModelConfig
	if config.Routing.TaskOverrides != nil {
		if taskModelID, ok := config.Routing.TaskOverrides[task]; ok && taskModelID != "" {
			if m, ok := modelMap[taskModelID]; ok {
				baseRoutedModel = &m
			}
		}
	}
	if baseRoutedModel == nil {
		defaultID := config.Routing.DefaultModelID
		if defaultID == "" && len(config.Models) > 0 {
			defaultID = config.Models[0].ID
		}
		if defaultID != "" {
			if m, ok := modelMap[defaultID]; ok {
				baseRoutedModel = &m
			}
		}
	}

	var candidates []candidateModel

	// 1. Context Override (ID or legacy name)
	if ctx.OverrideModelID != "" {
		if m, ok := modelMap[ctx.OverrideModelID]; ok {
			candidates = append(candidates, candidateModel{model: m, tier: "override"})
		} else if baseRoutedModel != nil {
			// Legacy name override (W1)
			candidates = append(candidates, candidateModel{
				model:        *baseRoutedModel,
				tier:         "override(name)",
				nameOverride: ctx.OverrideModelID,
			})
		}
	}

	// 2. Task Override
	if config.Routing.TaskOverrides != nil {
		if taskModelID, ok := config.Routing.TaskOverrides[task]; ok && taskModelID != "" {
			if m, ok := modelMap[taskModelID]; ok {
				candidates = append(candidates, candidateModel{model: m, tier: fmt.Sprintf("task_override(%s)", task)})
			}
		}
	}

	// 2b. Inherited Task Override
	if inheritedTask, ok := TaskRoutingFallback[task]; ok && config.Routing.TaskOverrides != nil {
		if inheritedModelID, has := config.Routing.TaskOverrides[inheritedTask]; has && inheritedModelID != "" {
			if m, ok := modelMap[inheritedModelID]; ok {
				candidates = append(candidates, candidateModel{model: m, tier: fmt.Sprintf("inherited_task_override(%s->%s)", task, inheritedTask)})
			}
		}
	}

	// 3. Default Model
	defaultID := config.Routing.DefaultModelID
	if defaultID == "" && len(config.Models) > 0 {
		defaultID = config.Models[0].ID
	}
	if defaultID != "" {
		if m, ok := modelMap[defaultID]; ok {
			candidates = append(candidates, candidateModel{model: m, tier: "default_model"})
		}
	}

	// 4. Fallback Model
	if config.Routing.FallbackModelID != "" {
		if m, ok := modelMap[config.Routing.FallbackModelID]; ok {
			candidates = append(candidates, candidateModel{model: m, tier: "fallback_model", isFallback: true})
		}
	}

	// Evaluate candidates in priority order
	seen := make(map[string]bool)
	var skipReasons []string

	req := taskCapabilityRequirements[task]

	for _, cand := range candidates {
		dedupKey := fmt.Sprintf("%s::%s", cand.model.ID, cand.nameOverride)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		cred, credExists := credMap[cand.model.CredentialID]
		if !credExists || cred.IsManaged {
			skipReasons = append(skipReasons, fmt.Sprintf("%s (%s): missing valid credential", cand.tier, cand.model.ID))
			continue
		}

		if !kernel.DefaultRegistry.Has(cred.Provider) {
			skipReasons = append(skipReasons, fmt.Sprintf("%s (%s): provider %s not registered", cand.tier, cand.model.ID, cred.Provider))
			continue
		}

		effectiveModel := cand.model.Model
		if cand.nameOverride != "" {
			effectiveModel = cand.nameOverride
		}

		module, _ := kernel.DefaultRegistry.Get(cred.Provider)
		caps := module.Capabilities(effectiveModel)

		// Check task capability requirements
		if req != nil && !req.satisfied(caps) {
			skipReasons = append(skipReasons, fmt.Sprintf("%s (%s): model %s lacks required capability %s", cand.tier, cand.model.ID, effectiveModel, req.name))
			continue
		}

		reason := fmt.Sprintf("resolved via %s", cand.tier)
		if len(skipReasons) > 0 {
			reason = fmt.Sprintf("%s -> %s", strings.Join(skipReasons, "; "), reason)
		}

		verdict := RoutingVerdict{
			ModelID:      cand.model.ID,
			Reason:       reason,
			UsedFallback: cand.isFallback,
		}
		if cand.nameOverride != "" {
			verdict.ModelName = cand.nameOverride
		} else if ctx.OverrideModelName != "" {
			verdict.ModelName = ctx.OverrideModelName
		}
		return verdict
	}

	return RoutingVerdict{
		ModelID: "",
		Reason:  fmt.Sprintf("no capable model candidate for task %s: %s", task, strings.Join(skipReasons, "; ")),
	}
}

// ResolveFallback identifies the eligible fallback model for the requested task.
func (s *StaticTaskStrategy) ResolveFallback(task byok.LlmTask, config *byok.BYOKConfig) RoutingVerdict {
	if config == nil || config.Routing.FallbackModelID == "" {
		return RoutingVerdict{Reason: "no fallback model configured"}
	}

	req := taskCapabilityRequirements[task]

	for _, m := range config.Models {
		if m.ID == config.Routing.FallbackModelID {
			for _, c := range config.Credentials {
				if c.ID == m.CredentialID {
					if !kernel.DefaultRegistry.Has(c.Provider) {
						return RoutingVerdict{Reason: fmt.Sprintf("fallback provider %s not registered", c.Provider)}
					}
					module, _ := kernel.DefaultRegistry.Get(c.Provider)
					caps := module.Capabilities(m.Model)

					if req != nil && !req.satisfied(caps) {
						return RoutingVerdict{Reason: fmt.Sprintf("fallback model %s lacks capability %s", m.Model, req.name)}
					}

					return RoutingVerdict{
						ModelID:      m.ID,
						Reason:       "fallback candidate eligible",
						UsedFallback: true,
					}
				}
			}
		}
	}

	return RoutingVerdict{Reason: "fallback model not found or invalid"}
}
