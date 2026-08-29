package intoto

import (
	"time"

	"github.com/google/uuid"
)


const (
	StatementTypeV1     = "https://in-toto.io/Statement/v1"
	PredicateTypeReview = "https://scandrix.dev/attestations/code-review/v1"
	DSSEPayloadType     = "application/vnd.in-toto+json"
)

// ResourceDescriptor describes an artifact subject (commit SHA or build package).
type ResourceDescriptor struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"` // e.g. {"sha1": "...", "sha256": "..."}
}

// ReviewDecision classifies the attestation verdict.
type ReviewDecision string

const (
	DecisionApproved         ReviewDecision = "APPROVED"
	DecisionChangesRequested ReviewDecision = "CHANGES_REQUESTED"
	DecisionBlockedCritical  ReviewDecision = "BLOCKED_CRITICAL"
)

// ReviewAttestationPredicate encapsulates the verifiable review assertions.
type ReviewAttestationPredicate struct {
	WorkspaceID           uuid.UUID          `json:"workspace_id"`
	RepoNamespace         string             `json:"repo_namespace"`
	CommitSHA             string             `json:"commit_sha"`
	PullRequestNumber     int                `json:"pull_request_number"`
	Decision              ReviewDecision     `json:"decision"`
	TotalFindings         int                `json:"total_findings"`
	CriticalCount         int                `json:"critical_count"`
	HighCount             int                `json:"high_count"`
	ConsensusScore        float64            `json:"consensus_score"`
	EvaluatedRules        []string           `json:"evaluated_rules"`
	AttestedAt            time.Time          `json:"attested_at"`
	ReviewerAgentIdentity string             `json:"reviewer_agent_identity"`
}

// InTotoStatement represents the canonical in-toto v1 statement structure.
type InTotoStatement struct {
	Type          string                     `json:"_type"`
	Subject       []ResourceDescriptor       `json:"subject"`
	PredicateType string                     `json:"predicateType"`
	Predicate     ReviewAttestationPredicate `json:"predicate"`
}

// DSSESignature holds a signature over the PAE-encoded DSSE payload.
type DSSESignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"` // Base64 encoded Ed25519 signature
}

// DSSEEnvelope represents the Dead Simple Signing Envelope.
type DSSEEnvelope struct {
	PayloadType string          `json:"payloadType"`
	Payload     string          `json:"payload"` // Base64 encoded JSON
	Signatures  []DSSESignature `json:"signatures"`
}
