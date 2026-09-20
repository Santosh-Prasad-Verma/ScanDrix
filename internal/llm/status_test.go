// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/llm/byok"
	_ "github.com/scandrix/backend/internal/llm/providers/all"
)

func TestDescribeLLMConfigStatus_ConfiguredBYOK(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{
				ID:       "cred-1",
				Provider: "openai",
				APIKey:   "enc-api-key-test",
			},
		},
		Models: []byok.BYOKModelConfig{
			{
				ID:           "model-1",
				CredentialID: "cred-1",
				Model:        "gpt-5",
			},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "model-1",
		},
	}

	status := llm.DescribeLLMConfigStatus(cfg)
	if status.Source != llm.SourceBYOK {
		t.Errorf("expected source to be byok, got %s", status.Source)
	}
	if !status.BYOK.Configured {
		t.Errorf("expected BYOK.Configured to be true")
	}
	if status.BYOK.Model != "gpt-5" {
		t.Errorf("expected BYOK.Model to be gpt-5, got %s", status.BYOK.Model)
	}
	if len(status.Models) != 1 {
		t.Fatalf("expected 1 model status, got %d", len(status.Models))
	}
	if !status.Models[0].Resolvable {
		t.Errorf("expected model-1 to be resolvable")
	}
}

func TestIsBYOKSlotConfigured(t *testing.T) {
	if llm.IsBYOKSlotConfigured(nil) {
		t.Errorf("expected false for nil slot")
	}

	openaiSlot := &byok.NormalizedModel{
		Provider: byok.ProviderOpenAI,
		APIKey:   "sk-test",
	}
	if !llm.IsBYOKSlotConfigured(openaiSlot) {
		t.Errorf("expected true for OpenAI slot with API key")
	}

	bedrockSlot := &byok.NormalizedModel{
		Provider:       byok.ProviderAmazonBedrock,
		AWSBearerToken: "token-test",
	}
	if !llm.IsBYOKSlotConfigured(bedrockSlot) {
		t.Errorf("expected true for Bedrock slot with Bearer token")
	}

	emptySlot := &byok.NormalizedModel{
		Provider: byok.ProviderAnthropic,
	}
	if llm.IsBYOKSlotConfigured(emptySlot) {
		t.Errorf("expected false for slot with no auth")
	}
}
