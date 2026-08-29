package checker

import (
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SuggestionVerifier checks if a developer's subsequent commits resolved a finding.
type SuggestionVerifier struct{}

// NewSuggestionVerifier initializes the verification engine.
func NewSuggestionVerifier() *SuggestionVerifier {
	return &SuggestionVerifier{}
}

// Verify evaluates committed code against the original vulnerability and the AI suggestion.
func (v *SuggestionVerifier) Verify(
	findingID, wsID uuid.UUID,
	prNum int,
	originalCode, suggestedCode, committedCode string,
	regexRule string,
	commitSHA string,
) *SuggestionVerification {
	cleanOriginal := strings.TrimSpace(originalCode)
	cleanSuggested := strings.TrimSpace(suggestedCode)
	cleanCommitted := strings.TrimSpace(committedCode)

	res := &SuggestionVerification{
		FindingID:           findingID,
		WorkspaceID:         wsID,
		PullRequestNumber:   prNum,
		OriginalCode:        originalCode,
		SuggestedCode:       suggestedCode,
		CommittedCode:       committedCode,
		ResolutionCommitSHA: commitSHA,
		VerifiedAt:          time.Now().UTC(),
	}

	// 1. Check if the committed code still triggers the vulnerability rule
	stillVulnerable := false
	if regexRule != "" {
		if rx, err := regexp.Compile(regexRule); err == nil {
			if rx.MatchString(cleanCommitted) {
				stillVulnerable = true
			}
		}
	}
	res.StillVulnerable = stillVulnerable

	if stillVulnerable {
		res.Status = StatusPending
		res.SimilarityScore = 0.0
		return res
	}

	// 2. Vulnerability eliminated! Check if accepted exact vs manual
	if cleanCommitted == cleanSuggested {
		res.Status = StatusAcceptedExact
		res.SimilarityScore = 1.0
		return res
	}

	// 3. Manual fix: Compute token similarity between suggested and committed
	sim := calculateTokenSimilarity(cleanSuggested, cleanCommitted)
	res.SimilarityScore = sim

	if cleanCommitted != cleanOriginal {
		res.Status = StatusAcceptedManual
	} else {
		// Code did not change
		res.Status = StatusPending
	}

	return res
}

func calculateTokenSimilarity(s1, s2 string) float64 {
	tokens1 := strings.Fields(s1)
	tokens2 := strings.Fields(s2)

	if len(tokens1) == 0 && len(tokens2) == 0 {
		return 1.0
	}
	if len(tokens1) == 0 || len(tokens2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool)
	for _, t := range tokens1 {
		set1[t] = true
	}

	intersection := 0
	set2 := make(map[string]bool)
	for _, t := range tokens2 {
		set2[t] = true
		if set1[t] {
			intersection++
		}
	}

	union := len(set1) + len(set2) - intersection
	if union == 0 {
		return 0.0
	}

	ratio := float64(intersection) / float64(union)
	return math.Round(ratio*100) / 100
}
