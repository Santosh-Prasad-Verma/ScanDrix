package intoto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ephemeralProvenanceSeed     string
	ephemeralProvenanceSeedOnce sync.Once
)

func getRuntimeProvenanceSecret() string {
	ephemeralProvenanceSeedOnce.Do(func() {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		ephemeralProvenanceSeed = hex.EncodeToString(b)
	})
	return ephemeralProvenanceSeed
}

// ProvenanceAttestor produces and cryptographically verifies DSSE-wrapped in-toto & SLSA attestations.
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

// DeriveTenantKeypair generates a deterministic, cryptographically secure Ed25519 keypair for a workspace.
func DeriveTenantKeypair(wsID uuid.UUID, masterSecret string) (ed25519.PrivateKey, ed25519.PublicKey, string) {
	if masterSecret == "" {
		masterSecret = os.Getenv("PROVENANCE_SIGNING_KEY")
		if masterSecret == "" {
			masterSecret = os.Getenv("SCANDRIX_MASTER_ENCRYPTION_KEY")
			if masterSecret == "" {
				masterSecret = os.Getenv("SCANDRIX_ENCRYPTION_KEY")
				if masterSecret == "" {
					masterSecret = os.Getenv("KMS_MASTER_KEY")
				}
			}
		}
	}
	if masterSecret == "" {
		slog.Warn("PROVENANCE_SIGNING_KEY not configured, using cryptographically generated ephemeral runtime secret (Master Rule 1.1)")
		masterSecret = getRuntimeProvenanceSecret()
	}
	seedData := fmt.Sprintf("scandrix:tenant-ed25519-signer:v1:%s:%s", wsID.String(), masterSecret)
	hash := sha512.Sum512([]byte(seedData))
	seed := hash[:32] // 32-byte Ed25519 seed

	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	keyID := fmt.Sprintf("scandrix:ed25519:%s", wsID.String()[:8])

	return privKey, pubKey, keyID
}

// ExportPublicKeyPEM encodes the Ed25519 public key as standard PKIX PEM for external CI/CD verification.
func ExportPublicKeyPEM(pubKey ed25519.PublicKey) (string, error) {
	pkixBytes, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed marshaling public key: %w", err)
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pkixBytes,
	}
	return string(pem.EncodeToMemory(block)), nil
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

// GenerateSLSAStatement constructs a SLSA Provenance v1.0 statement.
func (a *ProvenanceAttestor) GenerateSLSAStatement(pred ReviewAttestationPredicate, reviewID uuid.UUID, startTime time.Time) *SLSAInTotoStatement {
	if startTime.IsZero() {
		startTime = time.Now().UTC().Add(-1 * time.Minute)
	}
	finishTime := time.Now().UTC()

	slsaPredicate := SLSAProvenancePredicate{
		BuildDefinition: SLSABuildDefinition{
			BuildType: "https://scandrix.dev/slsa/code-review/v1",
			ExternalParameters: map[string]any{
				"repository":      pred.RepoNamespace,
				"commit_sha":      pred.CommitSHA,
				"pull_number":     pred.PullRequestNumber,
				"rules_evaluated": pred.EvaluatedRules,
			},
			InternalParameters: map[string]any{
				"workspace_id":    pred.WorkspaceID.String(),
				"review_id":       reviewID.String(),
				"consensus_score": pred.ConsensusScore,
			},
			ResolvedDependencies: []ResourceDescriptor{
				{
					Name: pred.RepoNamespace,
					Digest: map[string]string{
						"gitCommit": pred.CommitSHA,
					},
				},
			},
		},
		RunDetails: SLSARunDetails{
			Builder: SLSABuilder{
				ID: "https://scandrix.dev/builders/review-engine@v1",
				Version: map[string]string{
					"engine": "ScanDrix Go v1.25",
					"agent":  pred.ReviewerAgentIdentity,
				},
			},
			Metadata: SLSAMetadata{
				InvocationID: reviewID.String(),
				StartedOn:    startTime,
				FinishedOn:   finishTime,
			},
			Byproducts: []ResourceDescriptor{
				{
					Name: fmt.Sprintf("review-decision-%s", pred.Decision),
					Digest: map[string]string{
						"findingsCount": fmt.Sprintf("%d", pred.TotalFindings),
						"criticalCount": fmt.Sprintf("%d", pred.CriticalCount),
						"highCount":     fmt.Sprintf("%d", pred.HighCount),
					},
				},
			},
		},
	}

	return &SLSAInTotoStatement{
		Type: StatementTypeV1,
		Subject: []ResourceDescriptor{
			{
				Name: pred.RepoNamespace,
				Digest: map[string]string{
					"gitCommit": pred.CommitSHA,
				},
			},
		},
		PredicateType: PredicateTypeSLSA,
		Predicate:     slsaPredicate,
	}
}

// AttestAndSign serializes the statement, computes DSSE PAE, and signs with Ed25519.
func (a *ProvenanceAttestor) AttestAndSign(pred ReviewAttestationPredicate) (*DSSEEnvelope, error) {
	stmt := a.GenerateStatement(pred)
	return a.signStatement(stmt)
}

// AttestAndSignSLSA constructs and cryptographically signs a SLSA v1.0 provenance envelope.
func (a *ProvenanceAttestor) AttestAndSignSLSA(pred ReviewAttestationPredicate, reviewID uuid.UUID, startTime time.Time) (*DSSEEnvelope, error) {
	stmt := a.GenerateSLSAStatement(pred, reviewID, startTime)
	return a.signStatement(stmt)
}

func (a *ProvenanceAttestor) signStatement(stmt any) (*DSSEEnvelope, error) {
	payloadBytes, err := json.Marshal(stmt)
	if err != nil {
		return nil, fmt.Errorf("failed serializing statement: %w", err)
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

// VerifyEnvelope decodes and validates a DSSE envelope containing a review attestation.
func (a *ProvenanceAttestor) VerifyEnvelope(env *DSSEEnvelope, pubKey ed25519.PublicKey) (*InTotoStatement, error) {
	payloadBytes, err := a.verifyEnvelopeRaw(env, pubKey)
	if err != nil {
		return nil, err
	}

	var stmt InTotoStatement
	if err := json.Unmarshal(payloadBytes, &stmt); err != nil {
		return nil, fmt.Errorf("failed parsing attested in-toto statement: %w", err)
	}

	return &stmt, nil
}

// VerifySLSAEnvelope decodes and validates a DSSE envelope containing a SLSA v1.0 provenance statement.
func (a *ProvenanceAttestor) VerifySLSAEnvelope(env *DSSEEnvelope, pubKey ed25519.PublicKey) (*SLSAInTotoStatement, error) {
	payloadBytes, err := a.verifyEnvelopeRaw(env, pubKey)
	if err != nil {
		return nil, err
	}

	var stmt SLSAInTotoStatement
	if err := json.Unmarshal(payloadBytes, &stmt); err != nil {
		return nil, fmt.Errorf("failed parsing SLSA provenance statement: %w", err)
	}

	return &stmt, nil
}

// VerifyGenericEnvelope decodes and validates any DSSE envelope.
func (a *ProvenanceAttestor) VerifyGenericEnvelope(env *DSSEEnvelope, pubKey ed25519.PublicKey) (*GenericInTotoStatement, error) {
	payloadBytes, err := a.verifyEnvelopeRaw(env, pubKey)
	if err != nil {
		return nil, err
	}

	var stmt GenericInTotoStatement
	if err := json.Unmarshal(payloadBytes, &stmt); err != nil {
		return nil, fmt.Errorf("failed parsing generic statement: %w", err)
	}

	return &stmt, nil
}

func (a *ProvenanceAttestor) verifyEnvelopeRaw(env *DSSEEnvelope, pubKey ed25519.PublicKey) ([]byte, error) {
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

	return payloadBytes, nil
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
