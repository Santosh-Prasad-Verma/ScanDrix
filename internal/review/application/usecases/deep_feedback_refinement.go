package usecases

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FeedbackVote represents user feedback on an AI code suggestion.
type FeedbackVote string

const (
	VoteHelpful       FeedbackVote = "HELPFUL"
	VoteFalsePositive FeedbackVote = "FALSE_POSITIVE"
	VoteIrrelevant    FeedbackVote = "IRRELEVANT"
	VoteAlreadyKnown  FeedbackVote = "ALREADY_KNOWN"
	VoteTooStrict     FeedbackVote = "TOO_STRICT"
)

// UserSuggestionFeedback records explicit developer reaction to a specific finding.
type UserSuggestionFeedback struct {
	FeedbackID   uuid.UUID    `json:"feedback_id"`
	FindingID    uuid.UUID    `json:"finding_id"`
	RuleID       string       `json:"rule_id"`
	WorkspaceID  uuid.UUID    `json:"workspace_id"`
	AuthorEmail  string       `json:"author_email"`
	Vote         FeedbackVote `json:"vote"`
	Comments     string       `json:"comments,omitempty"`
	ReportedAt   time.Time    `json:"reported_at"`
	AffectedPath string       `json:"affected_path"`
	Snippet      string       `json:"snippet,omitempty"`
}

// RuleHealthMetrics tracks statistical performance and developer sentiment for a review rule.
type RuleHealthMetrics struct {
	RuleID            string       `json:"rule_id"`
	TotalFired        int          `json:"total_fired"`
	HelpfulCount      int          `json:"helpful_count"`
	FalsePositiveCount int         `json:"false_positive_count"`
	IrrelevantCount   int          `json:"irrelevant_count"`
	NoiseRatio        float64      `json:"noise_ratio"` // (FP + Irrelevant) / TotalFired
	PrecisionScore    float64      `json:"precision_score"` // Helpful / TotalFired
	RecommendedAction string       `json:"recommended_action"` // "HEALTHY", "NEEDS_TUNING", "AUTO_SUPPRESS"
	SynthesizedExclusion string    `json:"synthesized_exclusion,omitempty"`
}

// DeepFeedbackRefinementService collects feedback and automatically suggests negative pattern rules.
type DeepFeedbackRefinementService struct {
	mu           sync.RWMutex
	feedbacks    []UserSuggestionFeedback
	noiseWarnThresh float64
	noiseSuppressThresh float64
}

// NewDeepFeedbackRefinementService creates a feedback and refinement coordinator.
func NewDeepFeedbackRefinementService() *DeepFeedbackRefinementService {
	return &DeepFeedbackRefinementService{
		feedbacks:           make([]UserSuggestionFeedback, 0),
		noiseWarnThresh:     0.30, // >30% false positives -> needs tuning
		noiseSuppressThresh: 0.60, // >60% false positives -> auto suppress
	}
}

// IngestFeedback records an incoming developer reaction.
func (s *DeepFeedbackRefinementService) IngestFeedback(ctx context.Context, fb UserSuggestionFeedback) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if fb.FeedbackID == uuid.Nil {
		fb.FeedbackID = uuid.New()
	}
	if fb.ReportedAt.IsZero() {
		fb.ReportedAt = time.Now().UTC()
	}

	s.feedbacks = append(s.feedbacks, fb)
	return nil
}

// EvaluateRuleHealth computes telemetry, noise ratios, and auto-tuning recommendations for a given rule.
func (s *DeepFeedbackRefinementService) EvaluateRuleHealth(ctx context.Context, ruleID string) RuleHealthMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var helpful, fp, irrev int
	var fpSnippets []string

	for _, fb := range s.feedbacks {
		if fb.RuleID == ruleID {
			switch fb.Vote {
			case VoteHelpful:
				helpful++
			case VoteFalsePositive:
				fp++
				if fb.Snippet != "" {
					fpSnippets = append(fpSnippets, fb.Snippet)
				}
			case VoteIrrelevant, VoteAlreadyKnown, VoteTooStrict:
				irrev++
			}
		}
	}

	total := helpful + fp + irrev
	if total == 0 {
		return RuleHealthMetrics{
			RuleID:            ruleID,
			RecommendedAction: "INSUFFICIENT_DATA",
		}
	}

	noise := float64(fp+irrev) / float64(total)
	precision := float64(helpful) / float64(total)

	action := "HEALTHY"
	if noise >= s.noiseSuppressThresh {
		action = "AUTO_SUPPRESS"
	} else if noise >= s.noiseWarnThresh {
		action = "NEEDS_TUNING"
	}

	exclusionPattern := ""
	if len(fpSnippets) > 0 {
		exclusionPattern = synthesizeNegativeRegex(fpSnippets)
	}

	return RuleHealthMetrics{
		RuleID:               ruleID,
		TotalFired:           total,
		HelpfulCount:         helpful,
		FalsePositiveCount:   fp,
		IrrelevantCount:      irrev,
		NoiseRatio:           math.Round(noise*1000) / 1000,
		PrecisionScore:       math.Round(precision*1000) / 1000,
		RecommendedAction:    action,
		SynthesizedExclusion: exclusionPattern,
	}
}

// ListUnhealthyRules identifies all custom rules exceeding false positive noise thresholds.
func (s *DeepFeedbackRefinementService) ListUnhealthyRules(ctx context.Context) []RuleHealthMetrics {
	s.mu.RLock()
	rulesMap := make(map[string]bool)
	for _, fb := range s.feedbacks {
		rulesMap[fb.RuleID] = true
	}
	s.mu.RUnlock()

	var unhealthy []RuleHealthMetrics
	for ruleID := range rulesMap {
		metrics := s.EvaluateRuleHealth(ctx, ruleID)
		if metrics.RecommendedAction == "NEEDS_TUNING" || metrics.RecommendedAction == "AUTO_SUPPRESS" {
			unhealthy = append(unhealthy, metrics)
		}
	}

	sort.Slice(unhealthy, func(i, j int) bool {
		return unhealthy[i].NoiseRatio > unhealthy[j].NoiseRatio
	})

	return unhealthy
}

// synthesizeNegativeRegex extracts common tokens from false positive snippets to suggest regex exclusions.
func synthesizeNegativeRegex(snippets []string) string {
	if len(snippets) == 0 {
		return ""
	}

	tokenFreq := make(map[string]int)
	for _, snip := range snippets {
		// Tokenize on non-alphanumeric
		tokens := strings.FieldsFunc(snip, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
		})
		seenInSnip := make(map[string]bool)
		for _, tok := range tokens {
			tokLower := strings.ToLower(tok)
			if len(tokLower) >= 4 && !seenInSnip[tokLower] {
				seenInSnip[tokLower] = true
				tokenFreq[tokLower]++
			}
		}
	}

	var candidates []string
	for tok, count := range tokenFreq {
		if count >= len(snippets)/2 && count >= 2 {
			candidates = append(candidates, regexp.QuoteMeta(tok))
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	sort.Strings(candidates)
	return fmt.Sprintf("(?i)(?:%s)", strings.Join(candidates, "|"))
}
