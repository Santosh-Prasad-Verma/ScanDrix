// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System Unit Tests
// File: drixy_rule_summary_service_test.go
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"testing"

	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/infrastructure/adapters/services"
)

func TestDrixyRuleSummaryService_ComputeSourceHash(t *testing.T) {
	ruleText := "Always validate JWT expiry before processing claims"
	examples := []interfaces.DrixyRulesExample{
		{Snippet: "token.Verify()", IsCorrect: true},
		{Snippet: "token.ParseUnverified()", IsCorrect: false},
	}

	hash1 := services.ComputeSourceHash(ruleText, examples)
	hash2 := services.ComputeSourceHash(ruleText, examples)

	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}

	if hash1 != hash2 {
		t.Fatalf("expected deterministic hash equality, got %s vs %s", hash1, hash2)
	}

	// Change example and check hash changes
	examplesChanged := []interfaces.DrixyRulesExample{
		{Snippet: "token.Verify()", IsCorrect: true},
	}
	hash3 := services.ComputeSourceHash(ruleText, examplesChanged)
	if hash1 == hash3 {
		t.Fatalf("expected different hash when examples change")
	}
}

func TestExternalReferenceLoaderService(t *testing.T) {
	loader := services.NewExternalReferenceLoaderService()

	text := "Follow our guide in @file:internal/auth/tokens.go and @file:pkg/crypto/keys.go"
	refs := loader.DetectFileReferences(text)

	if len(refs) != 2 {
		t.Fatalf("expected 2 detected references, got %d", len(refs))
	}
	if refs[0] != "internal/auth/tokens.go" {
		t.Fatalf("expected internal/auth/tokens.go, got %s", refs[0])
	}
	if refs[1] != "pkg/crypto/keys.go" {
		t.Fatalf("expected pkg/crypto/keys.go, got %s", refs[1])
	}

	// Traversal security check
	err := loader.ValidateFileReference("../etc/passwd")
	if err == nil {
		t.Fatal("expected traversal path to be rejected")
	}

	errValid := loader.ValidateFileReference("internal/auth/tokens.go")
	if errValid != nil {
		t.Fatalf("expected valid path to pass, got: %v", errValid)
	}
}
