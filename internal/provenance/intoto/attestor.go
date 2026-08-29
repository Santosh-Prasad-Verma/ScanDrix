package intoto

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// ProvenanceAttestor produces and cryptographically verifies DSSE-wrapped in-toto attestations.
type ProvenanceAttestor struct {
	keyID   string
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

// NewProvenanceAttestor initializes the attestor with an Ed25519 keypair.
func NewProvenanceAttestor(keyID string, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) *ProvenanceAttestor {
	return &ProvenanceAttestor{
		keyID:   keyID,
		privKey: privKey,
		pubKey:  pubKey,
	}
}

// GenerateStatement constructs an in-toto v1 statement from review predicate data.
func (a *ProvenanceAttestor) GenerateStatement(pred ReviewAttestationPredicate) *InTotoStatement {
	if pred.AttestedAt.IsZero() {
		pred.AttestedAt = time.Now().UTC()
	}
	if pred.ReviewerAgentIdentity == "" {
		pred.ReviewerAgentIdentity = "scandrix-agent-consensus-v1"
	}

	return &InTotoStatement{
		Type: StatementTypeV1,
		Subject: []ResourceDescriptor{
			{
				Name: pred.RepoNamespace,
				Digest: map[string]string{
					"gitCommit": pred.CommitSHA,
				},
			},
		},
		PredicateType: PredicateTypeReview,
		Predicate:     pred,
	}
}

// AttestAndSign serializes the statement, computes DSSE PAE, and signs with Ed25519.
func (a *ProvenanceAttestor) AttestAndSign(pred ReviewAttestationPredicate) (*DSSEEnvelope, error) {
	stmt := a.GenerateStatement(pred)

	payloadBytes, err := json.Marshal(stmt)
	if err != nil {
		return nil, fmt.Errorf("failed serializing in-toto statement: %w", err)
	}

	// DSSE Pre-Authentication Encoding (PAE)
	pae := ComputeDSSEPAE(DSSEPayloadType, payloadBytes)
	sigBytes := ed25519.Sign(a.privKey, pae)

	return &DSSEEnvelope{
		PayloadType: DSSEPayloadType,
		Payload:     base64.StdEncoding.EncodeToString(payloadBytes),
		Signatures: []DSSESignature{
			{
				KeyID: a.keyID,
				Sig:   base64.StdEncoding.EncodeToString(sigBytes),
			},
		},
	}, nil
}

// VerifyEnvelope decodes and validates a DSSE envelope against the public key.
func (a *ProvenanceAttestor) VerifyEnvelope(env *DSSEEnvelope, pubKey ed25519.PublicKey) (*InTotoStatement, error) {
	if env == nil {
		return nil, fmt.Errorf("envelope is nil")
	}
	if env.PayloadType != DSSEPayloadType {
		return nil, fmt.Errorf("unsupported payload type: %s", env.PayloadType)
	}
	if len(env.Signatures) == 0 {
		return nil, fmt.Errorf("envelope contains no signatures")
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed decoding payload: %w", err)
	}

	pae := ComputeDSSEPAE(env.PayloadType, payloadBytes)

	// Verify at least one signature with provided public key
	verified := false
	for _, sigEntry := range env.Signatures {
		sigBytes, err := base64.StdEncoding.DecodeString(sigEntry.Sig)
		if err != nil {
			continue
		}
		if ed25519.Verify(pubKey, pae, sigBytes) {
			verified = true
			break
		}
	}

	if !verified {
		return nil, fmt.Errorf("cryptographic verification failed: invalid Ed25519 signature")
	}

	var stmt InTotoStatement
	if err := json.Unmarshal(payloadBytes, &stmt); err != nil {
		return nil, fmt.Errorf("failed parsing attested in-toto statement: %w", err)
	}

	return &stmt, nil
}

// ComputeDSSEPAE computes the canonical Pre-Authentication Encoding:
// "DSSEv1" + " " + len(type) + " " + type + " " + len(body) + " " + body
func ComputeDSSEPAE(payloadType string, payload []byte) []byte {
	header := fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(payload))
	pae := make([]byte, 0, len(header)+len(payload))
	pae = append(pae, []byte(header)...)
	pae = append(pae, payload...)
	return pae
}
