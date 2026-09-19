// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

import (
	"encoding/json"
	"testing"
)

func TestMigrateLegacyToV2_Idempotent(t *testing.T) {
	v2 := &BYOKConfig{
		Version: 2,
		Credentials: []BYOKCredential{
			{ID: "c1", Provider: "openai", APIKey: "cipher-123"},
		},
		Models: []BYOKModelConfig{
			{ID: "m1", CredentialID: "c1", Model: "gpt-5"},
		},
		Routing: BYOKRouting{DefaultModelID: "m1"},
	}
	blob, err := json.Marshal(v2)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	res, err := MigrateLegacyToV2(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Version != 2 || len(res.Credentials) != 1 || len(res.Models) != 1 {
		t.Errorf("expected unchanged v2 config, got: %+v", res)
	}
}

func TestMigrateLegacyToV2_DedupCredentials(t *testing.T) {
	// Encrypt a test key
	encKey, err := EncryptKey("sk-test-secret-key")
	if err != nil {
		t.Fatalf("encrypt error: %v", err)
	}

	legacy := LegacyConfig{
		Main: &LegacySlot{
			Provider: "openai",
			Model:    "gpt-5",
			APIKey:   encKey,
			BaseURL:  "https://api.openai.com/v1",
		},
		Fallback: &LegacySlot{
			Provider: "openai",
			Model:    "gpt-4o",
			APIKey:   encKey,
			BaseURL:  "https://api.openai.com/v1",
		},
	}
	blob, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	res, err := MigrateLegacyToV2(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Credentials) != 1 {
		t.Fatalf("expected 1 deduplicated credential, got %d", len(res.Credentials))
	}
	if len(res.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(res.Models))
	}
	if res.Models[0].CredentialID != res.Credentials[0].ID || res.Models[1].CredentialID != res.Credentials[0].ID {
		t.Errorf("both models must reference the same deduped credential")
	}
	if res.Routing.DefaultModelID != "model-main" {
		t.Errorf("expected defaultModelId to be model-main, got %s", res.Routing.DefaultModelID)
	}
}

func TestMigrateLegacyToV2_PromoteFallback(t *testing.T) {
	encKey, _ := EncryptKey("sk-fallback-only")

	legacy := LegacyConfig{
		Fallback: &LegacySlot{
			Provider: "anthropic",
			Model:    "claude-sonnet-4-5",
			APIKey:   encKey,
		},
	}
	blob, _ := json.Marshal(legacy)

	res, err := MigrateLegacyToV2(blob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Credentials) != 1 {
		t.Fatalf("expected 1 credential from promoted fallback, got %d", len(res.Credentials))
	}
	if len(res.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(res.Models))
	}
	if res.Models[0].Model != "claude-sonnet-4-5" {
		t.Errorf("expected model claude-sonnet-4-5, got %s", res.Models[0].Model)
	}
}

func TestMigrateLegacyToV2_EmptyOrUnusable(t *testing.T) {
	res, err := MigrateLegacyToV2([]byte("{}"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Version != 2 || len(res.Credentials) != 0 || len(res.Models) != 0 {
		t.Errorf("expected empty v2 config, got %+v", res)
	}
}
