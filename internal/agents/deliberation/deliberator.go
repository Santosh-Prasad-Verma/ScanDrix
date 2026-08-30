package deliberation

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// AgentDeliberator orchestrates the multi-agent code review round-table.
type AgentDeliberator struct {
	mu        sync.RWMutex
	consensus *ConsensusEngine
	personas  []PersonaProfile
}

// NewAgentDeliberator initializes the deliberation orchestrator.
func NewAgentDeliberator(threshold float64) *AgentDeliberator {
	return &AgentDeliberator{
		consensus: NewConsensusEngine(threshold),
		personas:  GetDefaultPersonas(),
	}
}

// DeliberateExec runs candidate proposal, peer critiques, and consensus adjudication.
func (d *AgentDeliberator) DeliberateExec(
	ctx context.Context,
	candidates []CandidateFinding,
	critiques []PeerCritique,
) ([]models.CodeFinding, []ConsensusDecision) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	decisions := d.consensus.Adjudicate(candidates, critiques)

	var acceptedFindings []models.CodeFinding
	for _, dec := range decisions {
		if !dec.IsFilteredOut && dec.ConsensusReached {
			acceptedFindings = append(acceptedFindings, dec.Finding)
		}
	}

	return acceptedFindings, decisions
}

// DeliberateMultiTurn runs the full 3-turn deliberation protocol across all specialized personas.
func (d *AgentDeliberator) DeliberateMultiTurn(
	ctx context.Context,
	patches []*diff.FilePatch,
	initialCandidates []CandidateFinding,
) (*MultiTurnDeliberationState, []models.CodeFinding) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	multiTurn := NewMultiTurnEngine(d.consensus.threshold)
	return multiTurn.ExecuteMultiTurn(ctx, patches, initialCandidates)
}

// ProposeFinding helper to construct candidate findings.
func ProposeFinding(
	persona AgentPersona,
	filePath string,
	startLine, endLine int,
	title string,
	severity models.FindingSeverity,
	confidence float64,
	reasoning, patch string,
) CandidateFinding {
	return CandidateFinding{
		ID:             uuid.New(),
		Persona:        persona,
		FilePath:       filePath,
		StartLine:      startLine,
		EndLine:        endLine,
		Title:          title,
		Severity:       severity,
		Confidence:     confidence,
		Reasoning:      reasoning,
		SuggestedPatch: patch,
		ProposedAt:     time.Now().UTC(),
	}
}

// ProposeCritique helper to construct peer critiques.
func ProposeCritique(
	candID uuid.UUID,
	reviewer AgentPersona,
	verdict CritiqueVerdict,
	rebuttal string,
	penalty float64,
) PeerCritique {
	return PeerCritique{
		CandidateID:       candID,
		ReviewerPersona:   reviewer,
		Verdict:           verdict,
		Rebuttal:          rebuttal,
		ConfidencePenalty: penalty,
		CritiquedAt:       time.Now().UTC(),
	}
}
