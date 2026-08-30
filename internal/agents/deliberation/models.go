package deliberation

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// AgentPersona identifies the specialized code review role.
type AgentPersona string

const (
	PersonaSecurityAuditor       AgentPersona = "security_auditor"
	PersonaPerformanceArchitect AgentPersona = "performance_architect"
	PersonaCleanCodeReviewer     AgentPersona = "clean_code_reviewer"
	PersonaDevilsAdvocate        AgentPersona = "devils_advocate"
	PersonaConcurrencyAuditor    AgentPersona = "concurrency_auditor"
	PersonaMemoryLeakSpecialist  AgentPersona = "memory_leak_specialist"
	PersonaSQLOptimizer          AgentPersona = "sql_optimizer"
)

// CandidateFinding is a finding proposed by a single agent persona.
type CandidateFinding struct {
	ID             uuid.UUID              `json:"id"`
	Persona        AgentPersona           `json:"persona"`
	FilePath       string                 `json:"file_path"`
	StartLine      int                    `json:"start_line"`
	EndLine        int                    `json:"end_line"`
	Title          string                 `json:"title"`
	Severity       models.FindingSeverity `json:"severity"`
	Confidence     float64                `json:"confidence"` // 0.0 to 1.0
	Reasoning      string                 `json:"reasoning"`
	SuggestedPatch string                 `json:"suggested_patch"`
	ProposedAt     time.Time              `json:"proposed_at"`
}

// CritiqueVerdict expresses an agent's peer review of a proposed finding.
type CritiqueVerdict string

const (
	VerdictAgree         CritiqueVerdict = "AGREE"
	VerdictDisagree      CritiqueVerdict = "DISAGREE"
	VerdictFalsePositive CritiqueVerdict = "FALSE_POSITIVE"
	VerdictDowngrade     CritiqueVerdict = "DOWNGRADE"
	VerdictReinforce     CritiqueVerdict = "REINFORCE"
)

// PeerCritique represents a cross-examination statement on a candidate finding.
type PeerCritique struct {
	CandidateID        uuid.UUID       `json:"candidate_id"`
	ReviewerPersona    AgentPersona    `json:"reviewer_persona"`
	Verdict            CritiqueVerdict `json:"verdict"`
	Rebuttal           string          `json:"rebuttal"`
	ConfidencePenalty  float64         `json:"confidence_penalty"` // 0.0 to 1.0 penalty
	CritiquedAt        time.Time       `json:"critiqued_at"`
}

// AgentRebuttal represents an agent's defense or adjustment in Turn 3.
type AgentRebuttal struct {
	CandidateID     uuid.UUID `json:"candidate_id"`
	Persona         AgentPersona `json:"persona"`
	AcceptedCritique bool      `json:"accepted_critique"`
	RevisedPatch    string    `json:"revised_patch,omitempty"`
	ConfidenceDelta float64   `json:"confidence_delta"`
	Explanation     string    `json:"explanation"`
}

// DeliberationRound encapsulates an individual turn of the deliberation protocol.
type DeliberationRound struct {
	RoundNumber int             `json:"round_number"`
	Name        string          `json:"name"` // "Proposal", "Cross-Examination", "Rebuttal"
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
	ItemCount   int             `json:"item_count"`
}

// MultiTurnDeliberationState tracks full trajectory across multi-turn deliberation.
type MultiTurnDeliberationState struct {
	SessionID   uuid.UUID           `json:"session_id"`
	Rounds      []DeliberationRound `json:"rounds"`
	Candidates  []CandidateFinding  `json:"candidates"`
	Critiques   []PeerCritique      `json:"critiques"`
	Rebuttals   []AgentRebuttal     `json:"rebuttals"`
	Decisions   []ConsensusDecision `json:"decisions"`
	Duration    time.Duration       `json:"duration"`
}

// ConsensusDecision is the finalized, multi-agent adjudicated review item.
type ConsensusDecision struct {
	Finding              models.CodeFinding `json:"finding"`
	FinalConfidence      float64            `json:"final_confidence"`
	ConsensusReached     bool               `json:"consensus_reached"`
	SupportingPersonas   []AgentPersona     `json:"supporting_personas"`
	Critiques            []PeerCritique     `json:"critiques"`
	Rebuttals            []AgentRebuttal    `json:"rebuttals,omitempty"`
	IsFilteredOut        bool               `json:"is_filtered_out"`
	FilterReason         string             `json:"filter_reason,omitempty"`
}
