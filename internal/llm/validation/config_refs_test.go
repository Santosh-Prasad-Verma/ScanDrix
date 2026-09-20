// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package validation_test

import (
	"testing"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/validation"
)

func TestValidateByokConfigRefs_Valid(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "cred-1", Provider: "openai"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "model-1", CredentialID: "cred-1", Model: "gpt-4o"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "model-1",
			TaskOverrides: map[byok.LlmTask]string{
				byok.TaskCodeReview: "model-1",
			},
		},
	}

	res := validation.ValidateByokConfigRefs(cfg)
	if !res.Valid {
		t.Fatalf("expected valid config, got errors: %v", res.Errors)
	}
}

func TestValidateByokConfigRefs_DanglingRefs(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "cred-1", Provider: "openai"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "model-1", CredentialID: "non-existent-cred", Model: "gpt-4o"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID:  "non-existent-model",
			FallbackModelID: "another-missing-model",
			TaskOverrides: map[byok.LlmTask]string{
				byok.TaskPRSummary: "missing-task-model",
			},
		},
	}

	res := validation.ValidateByokConfigRefs(cfg)
	if res.Valid {
		t.Fatal("expected invalid config with dangling references")
	}
	if len(res.Errors) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(res.Errors), res.Errors)
	}
}

func TestFindModelReferences(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{ID: "cred-1", Provider: "openai"},
		},
		Models: []byok.BYOKModelConfig{
			{ID: "model-1", CredentialID: "cred-1", Model: "gpt-4o"},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "model-1",
			TaskOverrides: map[byok.LlmTask]string{
				byok.TaskCodeReview:   "model-1",
				byok.TaskConversation: "model-1",
			},
		},
	}

	refs := validation.FindModelReferences(cfg, "model-1")
	if len(refs) != 3 {
		t.Fatalf("expected 3 references, got %d: %v", len(refs), refs)
	}
}
