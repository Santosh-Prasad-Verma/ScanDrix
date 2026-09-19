// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package invocation

import (
	"fmt"

	"github.com/scandrix/backend/internal/llm/byok"
)

// ModelIdentity captures the canonical identity of the model used for an LLM execution
// for spend accounting and observability.
type ModelIdentity struct {
	Model        string `json:"model"`
	IsBYOK       bool   `json:"is_byok"`
	BYOKModelID  string `json:"byok_model_id,omitempty"`
	CredentialID string `json:"credential_id,omitempty"`
}

// AgentModelIdentity derives the model identity quartet from a resolved slot.
// When slot is nil, attributes to the system managed default model.
func AgentModelIdentity(slot *byok.NormalizedModel) ModelIdentity {
	if slot == nil {
		return ModelIdentity{
			Model:  "system:default",
			IsBYOK: false,
		}
	}

	modelName := slot.Model
	if slot.Provider != "" {
		modelName = fmt.Sprintf("%s:%s", slot.Provider, slot.Model)
	}

	return ModelIdentity{
		Model:        modelName,
		IsBYOK:       true,
		BYOKModelID:  slot.BYOKModelID,
		CredentialID: slot.CredentialID,
	}
}
