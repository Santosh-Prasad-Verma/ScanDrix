// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package rulesengine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RuleFeedbackType indicates whether developer feedback was positive or negative.
type RuleFeedbackType string

const (
	FeedbackPositive RuleFeedbackType = "positive"
	FeedbackNegative RuleFeedbackType = "negative"
)

// RuleFeedbackReason provides structured rationale for developer feedback.
type RuleFeedbackReason string

const (
	ReasonFalsePositive  RuleFeedbackReason = "false_positive"
	ReasonIrrelevant     RuleFeedbackReason = "irrelevant"
	ReasonOutdatedRule   RuleFeedbackReason = "outdated_rule"
	ReasonPoorSuggestion RuleFeedbackReason = "poor_suggestion"
	ReasonHelpful        RuleFeedbackReason = "helpful"
	ReasonFixAccepted    RuleFeedbackReason = "fix_accepted"
)

// RuleHealthStatus indicates algorithmic and developer trust in a review rule.
type RuleHealthStatus string

const (
	HealthStatusHealthy         RuleHealthStatus = "healthy"
	HealthStatusNeedsRefinement RuleHealthStatus = "needs_refinement"
	HealthStatusAutoSuppressed  RuleHealthStatus = "auto_suppressed"
)

// RuleFeedbackRecord documents a single developer reaction to a rule finding.
type RuleFeedbackRecord struct {
	ID        string             `json:"id"`
	RuleID    string             `json:"rule_id"`
	UserID    string             `json:"user_id,omitempty"`
	ReviewID  string             `json:"review_id,omitempty"`
	Feedback  RuleFeedbackType   `json:"feedback"`
	Reason    RuleFeedbackReason `json:"reason"`
	Comment   string             `json:"comment,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
}

// RuleExecutionTelemetry aggregates execution counts and developer reactions for a rule.
type RuleExecutionTelemetry struct {
	RuleID                   string    `json:"rule_id"`
	Slug                     string    `json:"slug"`
	Evaluations              int64     `json:"evaluations"`
	Matches                  int64     `json:"matches"`
	FalsePositivesSuppressed int64     `json:"false_positives_suppressed"`
	PositiveFeedbackCount    int64     `json:"positive_feedback_count"`
	NegativeFeedbackCount    int64     `json:"negative_feedback_count"`
	AcceptedSuggestionsCount int64     `json:"accepted_suggestions_count"`
	DismissedSuggestionsCount int64    `json:"dismissed_suggestions_count"`
	LastEvaluatedAt          time.Time `json:"last_evaluated_at"`
	LastFeedbackAt           time.Time `json:"last_feedback_at,omitempty"`
}

// RuleHealthScore evaluates statistical precision and noise ratio of a rule.
type RuleHealthScore struct {
	RuleID                  string           `json:"rule_id"`
	Evaluations             int64            `json:"evaluations"`
	Matches                 int64            `json:"matches"`
	Precision               float64          `json:"precision"`                 // (positive + accepted) / total feedback
	DeveloperAcceptanceRate float64          `json:"developer_acceptance_rate"` // accepted / (accepted + dismissed)
	NoiseRatio              float64          `json:"noise_ratio"`               // negative feedback / total matches
	Status                  RuleHealthStatus `json:"status"`
	Recommendation          string           `json:"recommendation"`
}

// IRuleMetricsStore abstracts storage and analytics for rule telemetry and feedback.
type IRuleMetricsStore interface {
	RecordEvaluation(ctx context.Context, ruleID, slug string, matched, suppressed bool) error
	RecordFeedback(ctx context.Context, record RuleFeedbackRecord) error
	GetRuleTelemetry(ctx context.Context, ruleID string) (*RuleExecutionTelemetry, error)
	GetRuleHealth(ctx context.Context, ruleID string) (*RuleHealthScore, error)
	ListLowHealthRules(ctx context.Context, minPrecision float64) ([]*RuleHealthScore, error)
}

// InMemoryRuleMetricsStore provides a thread-safe, concurrent implementation of IRuleMetricsStore.
type InMemoryRuleMetricsStore struct {
	telemetryByRule map[string]*RuleExecutionTelemetry
	feedbackHistory map[string][]RuleFeedbackRecord
	mu              sync.RWMutex
}

// NewInMemoryRuleMetricsStore constructs a new in-memory metrics store.
func NewInMemoryRuleMetricsStore() *InMemoryRuleMetricsStore {
	return &InMemoryRuleMetricsStore{
		telemetryByRule: make(map[string]*RuleExecutionTelemetry),
		feedbackHistory: make(map[string][]RuleFeedbackRecord),
	}
}

// RecordEvaluation tracks a single rule execution.
func (s *InMemoryRuleMetricsStore) RecordEvaluation(ctx context.Context, ruleID, slug string, matched, suppressed bool) error {
	if ruleID == "" {
		return errors.New("ruleID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.telemetryByRule[ruleID]
	if !exists {
		t = &RuleExecutionTelemetry{
			RuleID: ruleID,
			Slug:   slug,
		}
		s.telemetryByRule[ruleID] = t
	}

	t.Evaluations++
	if matched {
		t.Matches++
	}
	if suppressed {
		t.FalsePositivesSuppressed++
	}
	t.LastEvaluatedAt = time.Now().UTC()
	return nil
}

// RecordFeedback stores developer feedback and updates telemetry counts.
func (s *InMemoryRuleMetricsStore) RecordFeedback(ctx context.Context, record RuleFeedbackRecord) error {
	if record.RuleID == "" {
		return errors.New("record.RuleID cannot be empty")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.feedbackHistory[record.RuleID] = append(s.feedbackHistory[record.RuleID], record)

	t, exists := s.telemetryByRule[record.RuleID]
	if !exists {
		t = &RuleExecutionTelemetry{
			RuleID: record.RuleID,
		}
		s.telemetryByRule[record.RuleID] = t
	}

	if record.Feedback == FeedbackPositive {
		t.PositiveFeedbackCount++
		if record.Reason == ReasonFixAccepted {
			t.AcceptedSuggestionsCount++
		}
	} else if record.Feedback == FeedbackNegative {
		t.NegativeFeedbackCount++
		t.DismissedSuggestionsCount++
	}

	t.LastFeedbackAt = record.CreatedAt
	return nil
}

// GetRuleTelemetry retrieves aggregated telemetry for a rule.
func (s *InMemoryRuleMetricsStore) GetRuleTelemetry(ctx context.Context, ruleID string) (*RuleExecutionTelemetry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.telemetryByRule[ruleID]
	if !exists {
		return nil, fmt.Errorf("no telemetry found for rule: %s", ruleID)
	}

	// Copy to prevent data race
	copied := *t
	return &copied, nil
}

// GetRuleHealth calculates precision, noise ratio, and health status for a rule.
func (s *InMemoryRuleMetricsStore) GetRuleHealth(ctx context.Context, ruleID string) (*RuleHealthScore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.telemetryByRule[ruleID]
	if !exists {
		return &RuleHealthScore{
			RuleID:         ruleID,
			Status:         HealthStatusHealthy,
			Recommendation: "Insufficient telemetry; maintaining active status",
		}, nil
	}

	totalFeedback := t.PositiveFeedbackCount + t.NegativeFeedbackCount
	var precision float64 = 1.0
	if totalFeedback > 0 {
		precision = float64(t.PositiveFeedbackCount) / float64(totalFeedback)
	}

	totalDecisions := t.AcceptedSuggestionsCount + t.DismissedSuggestionsCount
	var acceptanceRate float64 = 1.0
	if totalDecisions > 0 {
		acceptanceRate = float64(t.AcceptedSuggestionsCount) / float64(totalDecisions)
	}

	var noiseRatio float64 = 0.0
	if t.Matches > 0 {
		noiseRatio = float64(t.NegativeFeedbackCount) / float64(t.Matches)
	}

	status := HealthStatusHealthy
	rec := "Rule precision is healthy"

	// Flag if we have enough sample size (at least 5 feedback interactions)
	if totalFeedback >= 5 {
		if precision < 0.40 || noiseRatio > 0.60 {
			status = HealthStatusAutoSuppressed
			rec = "Rule auto-suppressed due to excessive noise (>60%) or low precision (<40%)"
		} else if precision < 0.70 || noiseRatio > 0.30 {
			status = HealthStatusNeedsRefinement
			rec = "Rule requires refinement: add negative patterns or clarify invariant spec"
		}
	}

	return &RuleHealthScore{
		RuleID:                  ruleID,
		Evaluations:             t.Evaluations,
		Matches:                 t.Matches,
		Precision:               precision,
		DeveloperAcceptanceRate: acceptanceRate,
		NoiseRatio:              noiseRatio,
		Status:                  status,
		Recommendation:          rec,
	}, nil
}

// ListLowHealthRules returns all rules falling below a minimum precision threshold.
func (s *InMemoryRuleMetricsStore) ListLowHealthRules(ctx context.Context, minPrecision float64) ([]*RuleHealthScore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []*RuleHealthScore
	for ruleID := range s.telemetryByRule {
		health, err := s.GetRuleHealth(ctx, ruleID)
		if err == nil && health.Precision < minPrecision && health.Evaluations >= 5 {
			res = append(res, health)
		}
	}
	return res, nil
}
