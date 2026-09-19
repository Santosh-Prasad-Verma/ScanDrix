// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"strings"

	_ "github.com/scandrix/backend/internal/llm/providers/all"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// vendorToProviderID maps managed enum vendor prefixes to provider registry IDs:
// openai -> openai, anthropic -> anthropic, google -> google_gemini, vertex -> google_vertex, novita -> novita
var vendorToProviderID = map[string]string{
	"openai":    "openai",
	"anthropic": "anthropic",
	"google":    "google_gemini",
	"vertex":    "google_vertex",
	"novita":    "novita",
}

// ManagedModelMaxInputTokens resolves the input token ceiling for a managed-catalog model ID
// (in the format "<vendor>:<model>", e.g. "google:gemini-2.5-pro") by querying the provider
// registry's Capabilities(model).MaxInputTokens.
//
// Returns nil for bare BYOK model strings without vendor prefix, unknown vendors, or models
// with no pinned window (where caller falls back to its default budget).
func ManagedModelMaxInputTokens(id string) *int {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}

	sep := strings.Index(trimmed, ":")
	if sep < 0 {
		return nil // bare BYOK model string — not a managed ID
	}

	vendor := strings.ToLower(trimmed[:sep])
	model := trimmed[sep+1:]

	providerID, ok := vendorToProviderID[vendor]
	if !ok {
		return nil
	}

	provider, found := kernel.Get(providerID)
	if !found {
		return nil
	}

	caps := provider.Capabilities(model)
	if caps.MaxInputTokens <= 0 {
		return nil
	}

	val := caps.MaxInputTokens
	return &val
}
