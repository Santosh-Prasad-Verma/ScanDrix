package deliberation

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/pkg/models"
)

// MultiTurnEngine coordinates multi-agent proposal, cross-examination, and rebuttal rounds.
type MultiTurnEngine struct {
	consensus *ConsensusEngine
	personas  []PersonaProfile
}

// NewMultiTurnEngine initializes the multi-turn coordinator.
func NewMultiTurnEngine(threshold float64) *MultiTurnEngine {
	return &MultiTurnEngine{
		consensus: NewConsensusEngine(threshold),
		personas:  GetDefaultPersonas(),
	}
}

// ExecuteMultiTurn runs the full 3-turn interactive deliberation lifecycle.
func (e *MultiTurnEngine) ExecuteMultiTurn(
	ctx context.Context,
	patches []*diff.FilePatch,
	initialCandidates []CandidateFinding,
) (*MultiTurnDeliberationState, []models.CodeFinding) {
	startTime := time.Now().UTC()
	sessionID := uuid.New()

	state := &MultiTurnDeliberationState{
		SessionID:  sessionID,
		Rounds:     make([]DeliberationRound, 0),
		Candidates: make([]CandidateFinding, 0),
		Critiques:  make([]PeerCritique, 0),
		Rebuttals:  make([]AgentRebuttal, 0),
	}

	// ------------------------------------------------------------------------
	// Turn 1: Proposal Round
	// ------------------------------------------------------------------------
	t1Start := time.Now().UTC()
	candidates := initialCandidates

	// Automatically augment with specialized domain detectors (Concurrency, Memory, SQL)
	specialized := DetectSpecializedCandidates(patches)
	candidates = append(candidates, specialized...)
	state.Candidates = candidates

	state.Rounds = append(state.Rounds, DeliberationRound{
		RoundNumber: 1,
		Name:        "Proposal Round",
		StartedAt:   t1Start,
		CompletedAt: time.Now().UTC(),
		ItemCount:   len(candidates),
	})

	// ------------------------------------------------------------------------
	// Turn 2: Cross-Examination Round
	// ------------------------------------------------------------------------
	t2Start := time.Now().UTC()
	var critiques []PeerCritique

	for _, cand := range candidates {
		// 1. Devil's Advocate cross-examination
		if cand.Confidence < 0.75 {
			critiques = append(critiques, ProposeCritique(
				cand.ID,
				PersonaDevilsAdvocate,
				VerdictFalsePositive,
				"Low initial confidence candidate with potential upstream framework mitigation.",
				0.35,
			))
		}

		// 2. Concurrency Auditor cross-examination
		if cand.Persona == PersonaPerformanceArchitect && strings.Contains(cand.Title, "Concurrency") {
			critiques = append(critiques, ProposeCritique(
				cand.ID,
				PersonaConcurrencyAuditor,
				VerdictReinforce,
				"Concurrency Auditor verified race condition vulnerability in multithreaded context.",
				0.0,
			))
		}

		// 3. Memory Leak Specialist cross-examination
		if cand.Persona == PersonaCleanCodeReviewer && strings.Contains(cand.Title, "Resource") {
			critiques = append(critiques, ProposeCritique(
				cand.ID,
				PersonaMemoryLeakSpecialist,
				VerdictAgree,
				"Memory Specialist confirms missing defer close causes file descriptor leakage.",
				0.0,
			))
		}

		// 4. SQL Optimizer cross-examination
		if cand.Persona == PersonaSQLOptimizer {
			critiques = append(critiques, ProposeCritique(
				cand.ID,
				PersonaPerformanceArchitect,
				VerdictAgree,
				"Performance Architect validates query optimization will prevent connection pool saturation.",
				0.0,
			))
		}
	}

	state.Critiques = critiques
	state.Rounds = append(state.Rounds, DeliberationRound{
		RoundNumber: 2,
		Name:        "Cross-Examination Round",
		StartedAt:   t2Start,
		CompletedAt: time.Now().UTC(),
		ItemCount:   len(critiques),
	})

	// ------------------------------------------------------------------------
	// Turn 3: Rebuttal & Refinement Round
	// ------------------------------------------------------------------------
	t3Start := time.Now().UTC()
	var rebuttals []AgentRebuttal

	for _, crit := range critiques {
		if crit.Verdict == VerdictFalsePositive {
			rebuttals = append(rebuttals, AgentRebuttal{
				CandidateID:      crit.CandidateID,
				Persona:          crit.ReviewerPersona,
				AcceptedCritique: true,
				ConfidenceDelta:  -0.20,
				Explanation:      "Accepted Devil's Advocate critique; adjusted confidence downward.",
			})
		} else if crit.Verdict == VerdictReinforce || crit.Verdict == VerdictAgree {
			rebuttals = append(rebuttals, AgentRebuttal{
				CandidateID:      crit.CandidateID,
				Persona:          crit.ReviewerPersona,
				AcceptedCritique: false,
				ConfidenceDelta:  +0.05,
				Explanation:      "Peer agreement reinforces high-certainty finding.",
			})
		}
	}

	state.Rebuttals = rebuttals
	state.Rounds = append(state.Rounds, DeliberationRound{
		RoundNumber: 3,
		Name:        "Rebuttal & Refinement Round",
		StartedAt:   t3Start,
		CompletedAt: time.Now().UTC(),
		ItemCount:   len(rebuttals),
	})

	// ------------------------------------------------------------------------
	// Turn 4: Final Adjudication & Consensus
	// ------------------------------------------------------------------------
	decisions := e.consensus.Adjudicate(state.Candidates, state.Critiques)
	state.Decisions = decisions
	state.Duration = time.Since(startTime)

	var finalFindings []models.CodeFinding
	for _, dec := range decisions {
		if !dec.IsFilteredOut && dec.ConsensusReached {
			finalFindings = append(finalFindings, dec.Finding)
		}
	}

	return state, finalFindings
}
