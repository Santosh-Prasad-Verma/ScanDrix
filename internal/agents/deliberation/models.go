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

// ConsensusDecision is the finalized, multi-agent adjudicated review item.
type ConsensusDecision struct {
	Finding              models.CodeFinding `json:"finding"`
	FinalConfidence      float64            `json:"final_confidence"`
	ConsensusReached     bool               `json:"consensus_reached"`
	SupportingPersonas   []AgentPersona     `json:"supporting_personas"`
	Critiques            []PeerCritique     `json:"critiques"`
	IsFilteredOut        bool               `json:"is_filtered_out"`
	FilterReason         string             `json:"filter_reason,omitempty"`
}
