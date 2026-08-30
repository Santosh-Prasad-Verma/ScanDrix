package proofoffix_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/scandrix/proofoffix"
)

func TestProofOfFixVerificationAndAttestation(t *testing.T) {
	ctx := context.Background()
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	verifier := proofoffix.NewVerifier("scandrix-remediation-engine", privKey)
	findingID := uuid.New()

	// 1. Valid patch
	validPatch := proofoffix.CandidatePatch{
		FindingID:       findingID,
		FilePath:        "pkg/auth/login.go",
		OriginalSnippet: `query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", id)`,
		ProposedSnippet: `query := "SELECT * FROM users WHERE id = $1"`,
		Language:        "go",
		CWEID:           "CWE-89",
	}

	attestation, err := verifier.VerifyPatch(ctx, validPatch, "")
	if err != nil {
		t.Fatalf("expected valid patch to verify: %v", err)
	}
	if !attestation.IsProved || attestation.SignatureHex == "" {
		t.Fatalf("expected cryptographic attestation on valid fix: %+v", attestation)
	}

	// 2. Syntax delimiter failure
	brokenPatch := proofoffix.CandidatePatch{
		FindingID:       findingID,
		FilePath:        "pkg/auth/login.go",
		ProposedSnippet: `func badSyntax(a int { return a }`, // missing closing paren
	}
	_, err = verifier.VerifyPatch(ctx, brokenPatch, "")
	if err == nil {
		t.Fatal("expected syntax check to fail with mismatched delimiter")
	}

	// 3. Secondary vulnerability injection failure
	secondaryVulnPatch := proofoffix.CandidatePatch{
		FindingID:       findingID,
		FilePath:        "pkg/auth/login.go",
		ProposedSnippet: `eval("processInput(" + input + ")")`,
	}
	_, err = verifier.VerifyPatch(ctx, secondaryVulnPatch, "")
	if err == nil {
		t.Fatal("expected rescan to fail on secondary vulnerability introduction")
	}
}
