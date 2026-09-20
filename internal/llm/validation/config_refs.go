// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package validation

import (
	"fmt"

	"github.com/scandrix/backend/internal/llm/byok"
)

// ByokRefValidationResult represents the outcome of a referential integrity check on BYOKConfig.
type ByokRefValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

var taskLabels = map[byok.LlmTask]string{
	byok.TaskCodeReview:         "the Code Review model",
	byok.TaskDrixyRulesReview:   "the Drixy Rules review model",
	byok.TaskRuleGeneration:     "the Drixy Rules generation model",
	byok.TaskBusinessValidation: "the Business Rules validation model",
	byok.TaskPRSummary:          "the PR Summary model",
	byok.TaskConversation:       "the Chat model",
}

// ValidateByokConfigRefs ensures write-time referential integrity of a BYOKConfig blob.
// Asserts that every model references an existing credential, and every routing target
// references an existing model. Rejects dangling references without leaking secret material.
func ValidateByokConfigRefs(config *byok.BYOKConfig) ByokRefValidationResult {
	if config == nil {
		return ByokRefValidationResult{Valid: true, Errors: nil}
	}

	var errs []string

	credentialIDs := make(map[string]bool)
	for _, c := range config.Credentials {
		if c.ID != "" {
			credentialIDs[c.ID] = true
		}
	}

	modelIDs := make(map[string]bool)
	for _, m := range config.Models {
		if m.ID != "" {
			modelIDs[m.ID] = true
		}
	}

	// 1. Every model must reference an existing credential
	for _, m := range config.Models {
		if m.CredentialID == "" || !credentialIDs[m.CredentialID] {
			modelName := m.ID
			if modelName == "" {
				modelName = "(missing id)"
			}
			credID := m.CredentialID
			if credID == "" {
				credID = "(none)"
			}
			errs = append(errs, fmt.Sprintf("Model %q references credentialId %q which does not resolve to any credential", modelName, credID))
		}
	}

	// 2. Routing references must point to an existing model
	routing := config.Routing
	if routing.DefaultModelID != "" && !modelIDs[routing.DefaultModelID] {
		errs = append(errs, fmt.Sprintf("routing.defaultModelId %q does not resolve to any model", routing.DefaultModelID))
	}
	if routing.FallbackModelID != "" && !modelIDs[routing.FallbackModelID] {
		errs = append(errs, fmt.Sprintf("routing.fallbackModelId %q does not resolve to any model", routing.FallbackModelID))
	}
	for task, modelID := range routing.TaskOverrides {
		if modelID != "" && !modelIDs[modelID] {
			errs = append(errs, fmt.Sprintf("routing.taskOverrides.%s %q does not resolve to any model", task, modelID))
		}
	}

	return ByokRefValidationResult{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
}

// FindModelReferences returns human-readable labels of active routing references pointing to modelID.
// Used by the model deletion guard to prevent orphaning active routing configurations.
func FindModelReferences(config *byok.BYOKConfig, modelID string) []string {
	if config == nil || modelID == "" {
		return nil
	}

	var refs []string
	routing := config.Routing

	if routing.DefaultModelID == modelID {
		refs = append(refs, "your organization default model")
	}
	if routing.FallbackModelID == modelID {
		refs = append(refs, "your fallback model")
	}
	for task, id := range routing.TaskOverrides {
		if id == modelID {
			label, ok := taskLabels[task]
			if !ok {
				label = fmt.Sprintf("the %s model", task)
			}
			refs = append(refs, label)
		}
	}

	return refs
}
