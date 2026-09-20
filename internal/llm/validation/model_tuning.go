// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package validation

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ModelTuningInput encapsulates the candidate tuning parameters for a model slot.
type ModelTuningInput struct {
	Provider        string
	Model           string
	Temperature     *float64
	ReasoningEffort string // "none" | "low" | "medium" | "high"
}

// ModelTuningIssue describes a configuration mismatch against a model's intrinsic constraints.
type ModelTuningIssue struct {
	Field   string `json:"field"` // "temperature" | "reasoning"
	Message string `json:"message"`
}

// ValidateModelTuning validates configured slot tuning (temperature & reasoning effort)
// against the model's intrinsic traits and policies in the provider module.
// Returns an empty slice when configuration is sound.
func ValidateModelTuning(input ModelTuningInput) []ModelTuningIssue {
	var issues []ModelTuningIssue

	if input.Provider == "" || !kernel.DefaultRegistry.Has(input.Provider) {
		return issues
	}

	module, ok := kernel.DefaultRegistry.Get(input.Provider)
	if !ok || module == nil {
		return issues
	}

	cfg := byok.NormalizedModel{
		Provider:        byok.BYOKProvider(input.Provider),
		Model:           input.Model,
		Temperature:     input.Temperature,
		ReasoningEffort: input.ReasoningEffort,
	}

	label := strings.TrimSpace(input.Model)
	if label == "" {
		label = "This model"
	}

	// 1. Temperature validation against model policy
	if input.Temperature != nil {
		policy := module.TemperaturePolicy(cfg)
		if policy != nil {
			pKind := policy.Kind
			if pKind == "" {
				pKind = policy.Mode
			}
			pValue := policy.Value
			if pValue == nil {
				pValue = policy.FixedValue
			}

			if pKind == kernel.TemperatureUnsupported {
				issues = append(issues, ModelTuningIssue{
					Field:   "temperature",
					Message: fmt.Sprintf("%s does not accept a temperature — the value you set won't be sent. Clear the temperature field.", label),
				})
			} else if pKind == kernel.TemperatureFixed && pValue != nil && *input.Temperature != *pValue {
				issues = append(issues, ModelTuningIssue{
					Field:   "temperature",
					Message: fmt.Sprintf("%s always reasons, so its temperature is fixed at %g; the %g you set won't be used. Set it to %g or leave it unset.", label, *pValue, *input.Temperature, *pValue),
				})
			}
		}
	}

	// 2. Reasoning effort validation: turning reasoning OFF on models that always think
	if input.ReasoningEffort == "none" {
		traits := module.ReasoningTraits(cfg)
		if traits.ThinksByDefault && !traits.CanDisableThinking {
			issues = append(issues, ModelTuningIssue{
				Field:   "reasoning",
				Message: fmt.Sprintf("%s always reasons and can't be turned off. Remove the \"off\" reasoning setting.", label),
			})
		}
	}

	return issues
}
