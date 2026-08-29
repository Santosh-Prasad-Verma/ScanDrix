package intoto_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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
	if verifiedStmt.Predicate.Decision != intoto.DecisionApproved {
		t.Fatalf("expected decision APPROVED, got %s", verifiedStmt.Predicate.Decision)
	}
	if verifiedStmt.Predicate.ConsensusScore != 0.98 {
		t.Fatalf("expected consensus score 0.98, got %f", verifiedStmt.Predicate.ConsensusScore)
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

func TestDSSEPAEEncoding(t *testing.T) {
	pae := intoto.ComputeDSSEPAE("test-type", []byte("hello"))
	expected := "DSSEv1 9 test-type 5 hello"
	if string(pae) != expected {
		t.Fatalf("PAE mismatch: got %q, want %q", string(pae), expected)
	}
}
