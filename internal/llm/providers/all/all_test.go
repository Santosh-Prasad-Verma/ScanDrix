// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package all

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

func TestAllProvidersRegistered(t *testing.T) {
	expectedProviders := []string{
		"openai",
		"openai_compatible",
		"anthropic",
		"anthropic_compatible",
		"google_gemini",
		"gemini",
		"google",
		"google_vertex",
		"vertex",
		"amazon_bedrock",
		"bedrock",
		"open_router",
		"openrouter",
		"novita",
		"moonshot",
		"zai",
		"azure",
	}

	for _, p := range expectedProviders {
		mod, ok := kernel.Get(p)
		if !ok || mod == nil {
			t.Errorf("expected provider or alias %q to be registered in kernel", p)
		}
	}
}
