package deliberation

import (
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

const (
	DefaultConsensusThreshold = 0.70
)

// ConsensusEngine aggregates multi-agent findings and peer critiques into final decisions.
type ConsensusEngine struct {
	threshold float64
}

// NewConsensusEngine initializes the adjudicator.
func NewConsensusEngine(threshold float64) *ConsensusEngine {
	if threshold <= 0 {
		threshold = DefaultConsensusThreshold
	}
	return &ConsensusEngine{threshold: threshold}
}

// Adjudicate reconciles proposed findings, applies peer critiques, and filters false positives.
func (e *ConsensusEngine) Adjudicate(candidates []CandidateFinding, critiques []PeerCritique) []ConsensusDecision {
	// 1. Index critiques by candidate ID
	critiqueMap := make(map[uuid.UUID][]PeerCritique)
	for _, c := range critiques {
		critiqueMap[c.CandidateID] = append(critiqueMap[c.CandidateID], c)
	}

	// 2. Group candidates by file and overlapping line boundaries
	groups := groupOverlappingCandidates(candidates)

	var decisions []ConsensusDecision

	for _, grp := range groups {
		dec := e.adjudicateGroup(grp, critiqueMap)
		decisions = append(decisions, dec)
	}

	return decisions
}

func (e *ConsensusEngine) adjudicateGroup(grp []CandidateFinding, critiqueMap map[uuid.UUID][]PeerCritique) ConsensusDecision {
	if len(grp) == 0 {
		return ConsensusDecision{IsFilteredOut: true, FilterReason: "empty_group"}
	}

	// Sort candidates by initial confidence descending
	sort.Slice(grp, func(i, j int) bool {
		return grp[i].Confidence > grp[j].Confidence
	})
	primary := grp[0]

	var supportingPersonas []AgentPersona
	var allCritiques []PeerCritique

	weightedSum := 0.0
	totalWeight := 0.0

	for _, cand := range grp {
		supportingPersonas = append(supportingPersonas, cand.Persona)
		weight := PersonaWeight(cand.Persona)
		weightedSum += cand.Confidence * weight
		totalWeight += weight

		// Gather critiques on this candidate
		if crits, ok := critiqueMap[cand.ID]; ok {
			allCritiques = append(allCritiques, crits...)
		}
	}

	baseConfidence := 0.0
	if totalWeight > 0 {
		baseConfidence = weightedSum / totalWeight
	}

	// Apply peer critiques and penalties
	penaltySum := 0.0
	for _, crit := range allCritiques {
		critWeight := PersonaWeight(crit.ReviewerPersona)
		switch crit.Verdict {
		case VerdictFalsePositive:
			penalty := crit.ConfidencePenalty
			if penalty <= 0 {
				penalty = 0.40 // default false positive penalty
			}
			penaltySum += penalty * critWeight
		case VerdictDisagree:
			penaltySum += 0.20 * critWeight
		case VerdictAgree:
			baseConfidence += 0.05 // small reinforcement bonus
		}
	}

	finalConfidence := math.Max(0.0, math.Min(1.0, baseConfidence-penaltySum))
	finalConfidence = math.Round(finalConfidence*100) / 100

	finding := models.CodeFinding{
		ID:          uuid.New(),
		FilePath:    primary.FilePath,
		StartLine:   primary.StartLine,
		EndLine:     primary.EndLine,
		Title:       primary.Title,
		Severity:    primary.Severity,
		Category:    string(primary.Persona),
		Description: primary.Reasoning,
		Remediation: primary.SuggestedPatch,
	}

	isFiltered := false
	filterReason := ""
	consensusReached := false

	if finalConfidence < e.threshold {
		isFiltered = true
		filterReason = fmt.Sprintf("confidence %0.2f fell below consensus threshold %0.2f", finalConfidence, e.threshold)
	} else {
		consensusReached = true
	}

	return ConsensusDecision{
		Finding:            finding,
		FinalConfidence:    finalConfidence,
		ConsensusReached:   consensusReached,
		SupportingPersonas: supportingPersonas,
		Critiques:          allCritiques,
		IsFilteredOut:      isFiltered,
		FilterReason:       filterReason,
	}
}

func groupOverlappingCandidates(candidates []CandidateFinding) [][]CandidateFinding {
	var groups [][]CandidateFinding

	for _, cand := range candidates {
		placed := false
		for i, grp := range groups {
			if grp[0].FilePath == cand.FilePath && linesOverlap(grp[0].StartLine, grp[0].EndLine, cand.StartLine, cand.EndLine) {
				groups[i] = append(groups[i], cand)
				placed = true
				break
			}
		}
		if !placed {
			groups = append(groups, []CandidateFinding{cand})
		}
	}

	return groups
}

func linesOverlap(s1, e1, s2, e2 int) bool {
	// Expand by 2 lines for surrounding context overlap
	return !(e1+2 < s2 || e2+2 < s1)
}
