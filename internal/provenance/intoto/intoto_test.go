package intoto_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/provenance/intoto"
)

func TestInTotoDSSEAttestationAndVerification(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating Ed25519 keypair: %v", err)
	}

	keyID := "scandrix-ed25519-key-2026"
	attestor := intoto.NewProvenanceAttestor(keyID, privKey, pubKey)

	wsID := uuid.New()
	pred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           wsID,
		RepoNamespace:         "acme/payment-service",
		CommitSHA:             "9f83f51d14828f53a80e0ec095f0067b3014177a",
		PullRequestNumber:     142,
		Decision:              intoto.DecisionApproved,
		TotalFindings:         0,
		CriticalCount:         0,
		HighCount:             0,
		ConsensusScore:        0.98,
		EvaluatedRules:        []string{"SEC001_SQL_INJECTION", "SEC004_HARDCODED_SECRETS"},
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-agent-consensus-v1",
	}

	// 1. Attest and Sign
	env, err := attestor.AttestAndSign(pred)
	if err != nil {
		t.Fatalf("attest and sign failed: %v", err)
	}

	if env.PayloadType != intoto.DSSEPayloadType {
		t.Fatalf("unexpected payload type: %s", env.PayloadType)
	}
	if len(env.Signatures) != 1 || env.Signatures[0].KeyID != keyID {
		t.Fatalf("unexpected signatures: %+v", env.Signatures)
	}

	// 2. Verify DSSE Envelope with Public Key
	verifiedStmt, err := attestor.VerifyEnvelope(env, pubKey)
	if err != nil {
		t.Fatalf("verification failed on valid envelope: %v", err)
	}

	if verifiedStmt.Type != intoto.StatementTypeV1 {
		t.Fatalf("unexpected statement type: %s", verifiedStmt.Type)
	}
	if len(verifiedStmt.Subject) != 1 || verifiedStmt.Subject[0].Digest["gitCommit"] != pred.CommitSHA {
		t.Fatalf("unexpected subject: %+v", verifiedStmt.Subject)
	}

	// 3. Tamper Resistance: Tampering with payload fails
	tamperedEnv := *env
	payloadBytes, _ := base64.StdEncoding.DecodeString(env.Payload)
	payloadBytes[len(payloadBytes)-10] ^= 0xFF // Flip bits in payload
	tamperedEnv.Payload = base64.StdEncoding.EncodeToString(payloadBytes)

	_, err = attestor.VerifyEnvelope(&tamperedEnv, pubKey)
	if err == nil {
		t.Fatal("tamper resistance failure: expected error on tampered payload, but verification succeeded")
	}

	// 4. Verifying with Wrong Public Key fails
	wrongPubKey, _, _ := ed25519.GenerateKey(rand.Reader)
	_, err = attestor.VerifyEnvelope(env, wrongPubKey)
	if err == nil {
		t.Fatal("expected signature verification failure with wrong public key")
	}
}

func TestSLSAProvenanceAttestationAndVerification(t *testing.T) {
	wsID := uuid.New()
	reviewID := uuid.New()
	privKey, pubKey, keyID := intoto.DeriveTenantKeypair(wsID, "secret_salt_123")
	attestor := intoto.NewProvenanceAttestor(keyID, privKey, pubKey)

	pred := intoto.ReviewAttestationPredicate{
		WorkspaceID:           wsID,
		RepoNamespace:         "acme/core-engine",
		CommitSHA:             "c0ffee1234567890abcdef1234567890abcdef12",
		PullRequestNumber:     42,
		Decision:              intoto.DecisionApproved,
		TotalFindings:         0,
		CriticalCount:         0,
		HighCount:             0,
		ConsensusScore:        1.0,
		EvaluatedRules:        []string{"SEC001_SQLI"},
		AttestedAt:            time.Now().UTC(),
		ReviewerAgentIdentity: "scandrix-agent-consensus-v1",
	}

	env, err := attestor.AttestAndSignSLSA(pred, reviewID, time.Now().UTC().Add(-30*time.Second))
	if err != nil {
		t.Fatalf("failed generating SLSA attestation: %v", err)
	}

	stmt, err := attestor.VerifySLSAEnvelope(env, pubKey)
	if err != nil {
		t.Fatalf("failed verifying SLSA envelope: %v", err)
	}

	if stmt.PredicateType != intoto.PredicateTypeSLSA {
		t.Fatalf("expected predicateType %s, got %s", intoto.PredicateTypeSLSA, stmt.PredicateType)
	}
}

func TestDeriveTenantKeypairAndPEMExport(t *testing.T) {
	wsID := uuid.New()
	master := "test_master_secret"

	priv1, pub1, keyID1 := intoto.DeriveTenantKeypair(wsID, master)
	priv2, pub2, keyID2 := intoto.DeriveTenantKeypair(wsID, master)

	if keyID1 != keyID2 || !pub1.Equal(pub2) || string(priv1) != string(priv2) {
		t.Fatal("expected deterministic key derivation for same workspace and master key")
	}

	pemStr, err := intoto.ExportPublicKeyPEM(pub1)
	if err != nil {
		t.Fatalf("failed exporting PEM: %v", err)
	}

	if !strings.HasPrefix(pemStr, "-----BEGIN PUBLIC KEY-----") {
		t.Fatalf("expected valid PEM header, got: %s", pemStr)
	}
}

func TestDSSEPAEEncoding(t *testing.T) {
	pae := intoto.ComputeDSSEPAE("test-type", []byte("hello"))
	expected := "DSSEv1 9 test-type 5 hello"
	if string(pae) != expected {
		t.Fatalf("PAE mismatch: got %q, want %q", string(pae), expected)
	}
}
