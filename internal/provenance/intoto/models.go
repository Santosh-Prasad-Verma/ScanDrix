package intoto

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatementTypeV1     = "https://in-toto.io/Statement/v1"
	PredicateTypeReview = "https://scandrix.dev/attestations/code-review/v1"
	PredicateTypeSLSA   = "https://slsa.dev/provenance/v1"
	DSSEPayloadType     = "application/vnd.in-toto+json"
)

// ResourceDescriptor describes an artifact subject (commit SHA or build package).
type ResourceDescriptor struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"` // e.g. {"gitCommit": "...", "sha256": "..."}
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
	WorkspaceID           uuid.UUID      `json:"workspace_id"`
	RepoNamespace         string         `json:"repo_namespace"`
	CommitSHA             string         `json:"commit_sha"`
	PullRequestNumber     int            `json:"pull_request_number"`
	Decision              ReviewDecision `json:"decision"`
	TotalFindings         int            `json:"total_findings"`
	CriticalCount         int            `json:"critical_count"`
	HighCount             int            `json:"high_count"`
	ConsensusScore        float64        `json:"consensus_score"`
	EvaluatedRules        []string       `json:"evaluated_rules"`
	AttestedAt            time.Time      `json:"attested_at"`
	ReviewerAgentIdentity string         `json:"reviewer_agent_identity"`
}

// SLSABuilder identifies the build/review runner.
type SLSABuilder struct {
	ID      string            `json:"id"`
	Version map[string]string `json:"version,omitempty"`
}

// SLSABuildDefinition records invocation inputs and dependencies.
type SLSABuildDefinition struct {
	BuildType            string               `json:"buildType"`
	ExternalParameters   map[string]any       `json:"externalParameters"`
	InternalParameters   map[string]any       `json:"internalParameters,omitempty"`
	ResolvedDependencies []ResourceDescriptor `json:"resolvedDependencies,omitempty"`
}

// SLSAMetadata records execution timing.
type SLSAMetadata struct {
	InvocationID string    `json:"invocationId,omitempty"`
	StartedOn    time.Time `json:"startedOn,omitempty"`
	FinishedOn   time.Time `json:"finishedOn,omitempty"`
}

// SLSARunDetails records verification results.
type SLSARunDetails struct {
	Builder    SLSABuilder          `json:"builder"`
	Metadata   SLSAMetadata         `json:"metadata"`
	Byproducts []ResourceDescriptor `json:"byproducts,omitempty"`
}

// SLSAProvenancePredicate complies with the SLSA v1.0 standard specification.
type SLSAProvenancePredicate struct {
	BuildDefinition SLSABuildDefinition `json:"buildDefinition"`
	RunDetails      SLSARunDetails      `json:"runDetails"`
}

// InTotoStatement represents the canonical in-toto v1 statement structure for code reviews.
type InTotoStatement struct {
	Type          string                     `json:"_type"`
	Subject       []ResourceDescriptor       `json:"subject"`
	PredicateType string                     `json:"predicateType"`
	Predicate     ReviewAttestationPredicate `json:"predicate"`
}

// SLSAInTotoStatement represents the canonical SLSA v1.0 statement structure.
type SLSAInTotoStatement struct {
	Type          string                  `json:"_type"`
	Subject       []ResourceDescriptor    `json:"subject"`
	PredicateType string                  `json:"predicateType"`
	Predicate     SLSAProvenancePredicate `json:"predicate"`
}

// GenericInTotoStatement represents a generic in-toto v1 statement structure.
type GenericInTotoStatement struct {
	Type          string               `json:"_type"`
	Subject       []ResourceDescriptor `json:"subject"`
	PredicateType string               `json:"predicateType"`
	Predicate     any                  `json:"predicate"`
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

// AttestationRecord represents a stored attestation with verification state.
type AttestationRecord struct {
	ID            uuid.UUID      `json:"id"`
	WorkspaceID   uuid.UUID      `json:"workspace_id"`
	ReviewID      uuid.UUID      `json:"review_id"`
	PredicateType string         `json:"predicate_type"`
	Decision      ReviewDecision `json:"decision"`
	KeyID         string         `json:"key_id"`
	Envelope      DSSEEnvelope   `json:"envelope"`
	CreatedAt     time.Time      `json:"created_at"`
}
