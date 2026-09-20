package services

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// SuggestionLifecycleState defines the strict 7-state lifecycle of code review suggestions.
type SuggestionLifecycleState string

const (
	LifecycleStateDiscovered SuggestionLifecycleState = "DISCOVERED"
	LifecycleStateValidated  SuggestionLifecycleState = "VALIDATED"
	LifecycleStateQueued     SuggestionLifecycleState = "QUEUED"
	LifecycleStatePublished  SuggestionLifecycleState = "PUBLISHED"
	LifecycleStateCommitted  SuggestionLifecycleState = "COMMITTED"
	LifecycleStateRejected   SuggestionLifecycleState = "REJECTED"
	LifecycleStateSuperseded SuggestionLifecycleState = "SUPERSEDED"
)

// SuggestionPriorityStatus mirrors canonical priority outcomes.
type SuggestionPriorityStatus string

const (
	PriorityStatusPrioritized             SuggestionPriorityStatus = "PRIORITIZED"
	PriorityStatusPrioritizedByClustering SuggestionPriorityStatus = "PRIORITIZED_BY_CLUSTERING"
	PriorityStatusDiscardedBySeverity     SuggestionPriorityStatus = "DISCARDED_BY_SEVERITY"
	PriorityStatusDiscardedByQuantity     SuggestionPriorityStatus = "DISCARDED_BY_QUANTITY"
	PriorityStatusDiscardedBySafeguard    SuggestionPriorityStatus = "DISCARDED_BY_SAFEGUARD"
)

// RejectionReason specifies why a developer dismissed a suggestion.
type RejectionReason string

const (
	RejectionReasonFalsePositive RejectionReason = "FALSE_POSITIVE"
	RejectionReasonIntentional   RejectionReason = "INTENTIONAL_DESIGN"
	RejectionReasonOutdated      RejectionReason = "OUTDATED_CODE"
	RejectionReasonLowPriority   RejectionReason = "LOW_PRIORITY"
	RejectionReasonDuplicate     RejectionReason = "DUPLICATE"
)

// LifecycleTransitionRecord records an immutable audit log entry for every state transition.
type LifecycleTransitionRecord struct {
	ID           string                   `json:"id"`
	SuggestionID string                   `json:"suggestion_id"`
	FromState    SuggestionLifecycleState `json:"from_state"`
	ToState      SuggestionLifecycleState `json:"to_state"`
	Actor        string                   `json:"actor"`
	Trigger      string                   `json:"trigger"`
	Reason       string                   `json:"reason,omitempty"`
	Timestamp    time.Time                `json:"timestamp"`
	Metadata     map[string]any           `json:"metadata,omitempty"`
}

// ManagedSuggestion encapsulates a domain code suggestion along with its lifecycle state and audit records.
type ManagedSuggestion struct {
	Suggestion         *domain.CodeSuggestion    `json:"suggestion"`
	CurrentState       SuggestionLifecycleState  `json:"current_state"`
	PriorityStatus     SuggestionPriorityStatus  `json:"priority_status"`
	AuditHistory       []LifecycleTransitionRecord `json:"audit_history"`
	RejectionReason    RejectionReason           `json:"rejection_reason,omitempty"`
	ClusterID          string                    `json:"cluster_id,omitempty"`
	IsParent           bool                      `json:"is_parent"`
	ParentSuggestionID string                    `json:"parent_suggestion_id,omitempty"`
	ChildCount         int                       `json:"child_count"`
	TargetCommitSHA    string                    `json:"target_commit_sha"`
	BaseCommitSHA      string                    `json:"base_commit_sha"`
	PublishedCommentID string                    `json:"published_comment_id,omitempty"`
	PublishedThreadID  string                    `json:"published_thread_id,omitempty"`
	ResolvedInCommit   string                    `json:"resolved_in_commit,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
	UpdatedAt          time.Time                 `json:"updated_at"`
}

// SuggestionLifecycleEvent is dispatched to outbox topics and webhooks.
type SuggestionLifecycleEvent struct {
	EventID        string                   `json:"event_id"`
	OrganizationID string                   `json:"organization_id"`
	RepositoryID   string                   `json:"repository_id"`
	PullNumber     int                      `json:"pull_number"`
	SuggestionID   string                   `json:"suggestion_id"`
	PreviousState  SuggestionLifecycleState `json:"previous_state"`
	NewState       SuggestionLifecycleState `json:"new_state"`
	PriorityStatus SuggestionPriorityStatus `json:"priority_status"`
	OccurredAt     time.Time                `json:"occurred_at"`
}

// OutboxEventListener handles lifecycle outbox notifications.
type OutboxEventListener interface {
	OnSuggestionLifecycleEvent(ctx context.Context, event SuggestionLifecycleEvent) error
}

// SeverityQuotas defines maximum allowed suggestions per severity tier.
type SeverityQuotas struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

// DiffHunkOffset represents the line shift resulting from new commits.
type DiffHunkOffset struct {
	FilePath string
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Delta    int
}

// SuggestionLifecycleEngine orchestrates transitions, DBSCAN clustering, and commit drift rebase.
type SuggestionLifecycleEngine struct {
	mu           sync.RWMutex
	suggestions  map[string]*ManagedSuggestion
	transitions  map[string][]LifecycleTransitionRecord
	listeners    []OutboxEventListener
}

// NewSuggestionLifecycleEngine constructs an empty in-memory lifecycle engine.
func NewSuggestionLifecycleEngine() *SuggestionLifecycleEngine {
	return &SuggestionLifecycleEngine{
		suggestions: make(map[string]*ManagedSuggestion),
		transitions: make(map[string][]LifecycleTransitionRecord),
	}
}

// RegisterOutboxListener attaches an asynchronous event listener for webhooks/outbox.
func (e *SuggestionLifecycleEngine) RegisterOutboxListener(l OutboxEventListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, l)
}

// RegisterDiscoveredSuggestion creates a new managed suggestion in DISCOVERED state.
func (e *SuggestionLifecycleEngine) RegisterDiscoveredSuggestion(
	s *domain.CodeSuggestion,
	targetCommitSHA, baseCommitSHA string,
) (*ManagedSuggestion, error) {
	if s == nil {
		return nil, errors.New("cannot register nil code suggestion")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	sugID := s.ID.String()
	now := time.Now().UTC()

	initialRecord := LifecycleTransitionRecord{
		ID:           uuid.New().String(),
		SuggestionID: sugID,
		FromState:    "",
		ToState:      LifecycleStateDiscovered,
		Actor:        "scandrix_deliberation_agent",
		Trigger:      "deliberation_consensus",
		Timestamp:    now,
	}

	managed := &ManagedSuggestion{
		Suggestion:      s,
		CurrentState:    LifecycleStateDiscovered,
		PriorityStatus:  PriorityStatusPrioritized,
		AuditHistory:    []LifecycleTransitionRecord{initialRecord},
		TargetCommitSHA: targetCommitSHA,
		BaseCommitSHA:   baseCommitSHA,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	e.suggestions[sugID] = managed
	e.transitions[sugID] = []LifecycleTransitionRecord{initialRecord}

	return managed, nil
}

// Transition advances a suggestion to a new state if the transition is allowed by the state machine.
func (e *SuggestionLifecycleEngine) Transition(
	ctx context.Context,
	sugID string,
	toState SuggestionLifecycleState,
	actor string,
	trigger string,
	reason string,
	meta map[string]any,
) (*ManagedSuggestion, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ms, ok := e.suggestions[sugID]
	if !ok {
		return nil, fmt.Errorf("suggestion %s not found in lifecycle registry", sugID)
	}

	fromState := ms.CurrentState
	if !isAllowedTransition(fromState, toState) {
		return nil, fmt.Errorf("illegal state transition from %s to %s for suggestion %s", fromState, toState, sugID)
	}

	now := time.Now().UTC()
	record := LifecycleTransitionRecord{
		ID:           uuid.New().String(),
		SuggestionID: sugID,
		FromState:    fromState,
		ToState:      toState,
		Actor:        actor,
		Trigger:      trigger,
		Reason:       reason,
		Timestamp:    now,
		Metadata:     meta,
	}

	ms.CurrentState = toState
	ms.UpdatedAt = now
	ms.AuditHistory = append(ms.AuditHistory, record)
	e.transitions[sugID] = append(e.transitions[sugID], record)

	event := SuggestionLifecycleEvent{
		EventID:        uuid.New().String(),
		SuggestionID:   sugID,
		PreviousState:  fromState,
		NewState:       toState,
		PriorityStatus: ms.PriorityStatus,
		OccurredAt:     now,
	}

	for _, l := range e.listeners {
		_ = l.OnSuggestionLifecycleEvent(ctx, event)
	}

	return ms, nil
}

// isAllowedTransition implements the 7-state directed graph rules.
func isAllowedTransition(from, to SuggestionLifecycleState) bool {
	if from == to {
		return true
	}

	switch from {
	case LifecycleStateDiscovered:
		return to == LifecycleStateValidated || to == LifecycleStateRejected || to == LifecycleStateSuperseded
	case LifecycleStateValidated:
		return to == LifecycleStateQueued || to == LifecycleStateRejected || to == LifecycleStateSuperseded
	case LifecycleStateQueued:
		return to == LifecycleStatePublished || to == LifecycleStateRejected || to == LifecycleStateSuperseded
	case LifecycleStatePublished:
		return to == LifecycleStateCommitted || to == LifecycleStateRejected || to == LifecycleStateSuperseded
	case LifecycleStateCommitted:
		return to == LifecycleStateSuperseded // e.g. reverted or follow-up push
	case LifecycleStateRejected:
		return to == LifecycleStateValidated // Re-opened by human override
	case LifecycleStateSuperseded:
		return false // Terminal state
	default:
		return false
	}
}

// GetManagedSuggestion retrieves a managed suggestion by ID.
func (e *SuggestionLifecycleEngine) GetManagedSuggestion(sugID string) (*ManagedSuggestion, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ms, ok := e.suggestions[sugID]
	return ms, ok
}

// ClusterDrixySuggestionsByRule groups suggestions generated from custom Drixy rules.
// Matches canonical rule clustering where the primary rule ID is elected as parent.
func (e *SuggestionLifecycleEngine) ClusterDrixySuggestionsByRule(
	suggestions []*ManagedSuggestion,
) []*ManagedSuggestion {
	if len(suggestions) <= 1 {
		return suggestions
	}

	// Group by primary rule ID
	ruleMap := make(map[string][]*ManagedSuggestion)
	var unclusterable []*ManagedSuggestion

	for _, ms := range suggestions {
		if ms.Suggestion == nil {
			unclusterable = append(unclusterable, ms)
			continue
		}

		s := ms.Suggestion
		ruleID := ""
		if len(s.BrokenRuleIDs) > 0 && strings.TrimSpace(s.BrokenRuleIDs[0]) != "" {
			ruleID = strings.TrimSpace(s.BrokenRuleIDs[0])
		} else if s.RuleID != "" {
			ruleID = strings.TrimSpace(s.RuleID)
		}

		// Also check label
		if ruleID == "" && strings.EqualFold(string(s.Category), "drixy_rules") && len(s.BrokenRuleIDs) > 0 {
			ruleID = s.BrokenRuleIDs[0]
		}

		if ruleID != "" {
			ruleMap[ruleID] = append(ruleMap[ruleID], ms)
		} else {
			unclusterable = append(unclusterable, ms)
		}
	}

	var result []*ManagedSuggestion

	for ruleID, group := range ruleMap {
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		// Elect parent with highest rank score
		sort.Slice(group, func(i, j int) bool {
			return group[i].Suggestion.RankScore > group[j].Suggestion.RankScore
		})

		parent := group[0]
		clusterID := fmt.Sprintf("cluster_%s_%s", ruleID, hex.EncodeToString([]byte(uuid.New().String())[:4]))
		parent.ClusterID = clusterID
		parent.IsParent = true
		parent.ChildCount = len(group) - 1
		result = append(result, parent)

		parentID := parent.Suggestion.ID.String()
		for _, child := range group[1:] {
			child.ClusterID = clusterID
			child.IsParent = false
			child.ParentSuggestionID = parentID
			child.PriorityStatus = PriorityStatusPrioritizedByClustering
			result = append(result, child)
		}
	}

	result = append(result, unclusterable...)
	return result
}

// ClusterSuggestionsBySpatialDensity groups adjacent suggestions in the same file using DBSCAN-like line distance.
func (e *SuggestionLifecycleEngine) ClusterSuggestionsBySpatialDensity(
	suggestions []*ManagedSuggestion,
	maxLineDistance int,
) []*ManagedSuggestion {
	if len(suggestions) <= 1 || maxLineDistance <= 0 {
		return suggestions
	}

	// Group by file path
	fileGroups := make(map[string][]*ManagedSuggestion)
	for _, s := range suggestions {
		if s.Suggestion != nil {
			fileGroups[s.Suggestion.GetFilePath()] = append(fileGroups[s.Suggestion.GetFilePath()], s)
		}
	}

	var output []*ManagedSuggestion

	for _, group := range fileGroups {
		if len(group) <= 1 {
			output = append(output, group...)
			continue
		}

		// Sort by start line
		sort.Slice(group, func(i, j int) bool {
			return group[i].Suggestion.GetStartLine() < group[j].Suggestion.GetStartLine()
		})

		var currentCluster []*ManagedSuggestion
		for _, item := range group {
			if len(currentCluster) == 0 {
				currentCluster = append(currentCluster, item)
				continue
			}

			prev := currentCluster[len(currentCluster)-1]
			dist := item.Suggestion.GetStartLine() - prev.Suggestion.GetEndLine()
			if dist <= maxLineDistance {
				currentCluster = append(currentCluster, item)
			} else {
				output = append(output, resolveSpatialCluster(currentCluster)...)
				currentCluster = []*ManagedSuggestion{item}
			}
		}

		if len(currentCluster) > 0 {
			output = append(output, resolveSpatialCluster(currentCluster)...)
		}
	}

	return output
}

func resolveSpatialCluster(cluster []*ManagedSuggestion) []*ManagedSuggestion {
	if len(cluster) == 1 {
		return cluster
	}

	// Pick highest severity/rankScore as parent
	sort.Slice(cluster, func(i, j int) bool {
		return cluster[i].Suggestion.RankScore > cluster[j].Suggestion.RankScore
	})

	parent := cluster[0]
	clusterID := fmt.Sprintf("spatial_%s_%d", parent.Suggestion.GetFilePath(), parent.Suggestion.GetStartLine())
	parent.ClusterID = clusterID
	parent.IsParent = true
	parent.ChildCount = len(cluster) - 1

	parentID := parent.Suggestion.ID.String()
	for _, child := range cluster[1:] {
		child.ClusterID = clusterID
		child.IsParent = false
		child.ParentSuggestionID = parentID
		child.PriorityStatus = PriorityStatusPrioritizedByClustering
	}

	return cluster
}

// PrioritizeBySeverityQuotas enforces maximum finding caps per severity level.
func (e *SuggestionLifecycleEngine) PrioritizeBySeverityQuotas(
	suggestions []*ManagedSuggestion,
	quotas SeverityQuotas,
) ([]*ManagedSuggestion, []*ManagedSuggestion) {
	var relatedChildren []*ManagedSuggestion
	var primaryList []*ManagedSuggestion

	// Separate clustered related children from parent candidates
	for _, s := range suggestions {
		if s.ParentSuggestionID != "" {
			relatedChildren = append(relatedChildren, s)
		} else {
			primaryList = append(primaryList, s)
		}
	}

	// Partition into severity buckets
	buckets := map[string][]*ManagedSuggestion{
		"critical": {},
		"high":     {},
		"medium":   {},
		"low":      {},
	}

	for _, s := range primaryList {
		sev := strings.ToLower(string(s.Suggestion.Severity))
		if _, ok := buckets[sev]; ok {
			buckets[sev] = append(buckets[sev], s)
		} else {
			buckets["low"] = append(buckets["low"], s)
		}
	}

	// Sort each bucket descending by rankScore
	for sev := range buckets {
		sort.Slice(buckets[sev], func(i, j int) bool {
			return buckets[sev][i].Suggestion.RankScore > buckets[sev][j].Suggestion.RankScore
		})
	}

	var accepted []*ManagedSuggestion
	var discarded []*ManagedSuggestion

	applyLimit := func(items []*ManagedSuggestion, limit int) {
		for idx, it := range items {
			if limit < 0 || idx < limit {
				it.PriorityStatus = PriorityStatusPrioritized
				accepted = append(accepted, it)
			} else {
				it.PriorityStatus = PriorityStatusDiscardedByQuantity
				discarded = append(discarded, it)
			}
		}
	}

	applyLimit(buckets["critical"], quotas.Critical)
	applyLimit(buckets["high"], quotas.High)
	applyLimit(buckets["medium"], quotas.Medium)
	applyLimit(buckets["low"], quotas.Low)

	// Re-attach related children whose parent was accepted
	acceptedParentIDs := make(map[string]bool)
	for _, a := range accepted {
		if a.Suggestion != nil {
			acceptedParentIDs[a.Suggestion.ID.String()] = true
		}
	}

	for _, child := range relatedChildren {
		if acceptedParentIDs[child.ParentSuggestionID] {
			child.PriorityStatus = PriorityStatusPrioritizedByClustering
			accepted = append(accepted, child)
		} else {
			child.PriorityStatus = PriorityStatusDiscardedByQuantity
			discarded = append(discarded, child)
		}
	}

	return accepted, discarded
}

// RebaseSuggestionsOnCommitDrift recalculates line numbers when subsequent commits change lines above suggestions.
func (e *SuggestionLifecycleEngine) RebaseSuggestionsOnCommitDrift(
	suggestions []*ManagedSuggestion,
	newCommitSHA string,
	offsets []DiffHunkOffset,
) ([]*ManagedSuggestion, []string) {
	var valid []*ManagedSuggestion
	var supersededIDs []string

	offsetMap := make(map[string][]DiffHunkOffset)
	for _, o := range offsets {
		offsetMap[o.FilePath] = append(offsetMap[o.FilePath], o)
	}

	for _, ms := range suggestions {
		if ms.Suggestion == nil {
			continue
		}

		s := ms.Suggestion
		fileOffsets, hasOffsets := offsetMap[s.GetFilePath()]
		if !hasOffsets {
			// No diff in this file; remains valid
			ms.TargetCommitSHA = newCommitSHA
			valid = append(valid, ms)
			continue
		}

		originalStart := s.GetStartLine()
		originalEnd := s.GetEndLine()
		hunkOverwritten := false
		cumulativeDelta := 0

		for _, off := range fileOffsets {
			// Check if new commit directly overwrites the suggestion range
			hunkEnd := off.OldStart + off.OldCount
			if !(originalEnd < off.OldStart || originalStart > hunkEnd) {
				hunkOverwritten = true
				break
			}

			// If diff is strictly above the suggestion, apply delta
			if off.OldStart <= originalStart {
				cumulativeDelta += off.Delta
			}
		}

		if hunkOverwritten {
			// Suggestion was modified or invalidated by new commit -> mark SUPERSEDED
			ms.CurrentState = LifecycleStateSuperseded
			supersededIDs = append(supersededIDs, s.ID.String())
		} else {
			// Shift lines cleanly
			newStart := originalStart + cumulativeDelta
			newEnd := originalEnd + cumulativeDelta
			s.StartLine = newStart
			s.EndLine = newEnd
			s.RelevantLinesStart = newStart
			s.RelevantLinesEnd = newEnd
			ms.TargetCommitSHA = newCommitSHA
			valid = append(valid, ms)
		}
	}

	return valid, supersededIDs
}

// ReconcileDeveloperReaction maps developer feedback from PR comments to lifecycle mutations.
func (e *SuggestionLifecycleEngine) ReconcileDeveloperReaction(
	ctx context.Context,
	sugID string,
	analysis DeveloperCommentAnalysis,
) (*ManagedSuggestion, error) {
	e.mu.Lock()
	ms, ok := e.suggestions[sugID]
	e.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("suggestion %s not found for reaction reconcile", sugID)
	}

	switch analysis.Intent {
	case IntentAgreement, IntentAlreadyFixed:
		return e.Transition(ctx, sugID, LifecycleStateCommitted, analysis.Author, "developer_accepted", "Fixed or agreed in review discussion", nil)
	case IntentDisagreement:
		ms.RejectionReason = RejectionReasonIntentional
		return e.Transition(ctx, sugID, LifecycleStateRejected, analysis.Author, "developer_disagreed", analysis.Body, nil)
	case IntentFalsePositiveReport:
		ms.RejectionReason = RejectionReasonFalsePositive
		return e.Transition(ctx, sugID, LifecycleStateRejected, analysis.Author, "developer_false_positive_report", analysis.Body, map[string]any{"mute_rule": analysis.ShouldMuteRule})
	default:
		return ms, nil
	}
}

// CalculateCompositeRankScore computes a multi-factor score from 0.0 to 100.0.
// Incorporates severity weight, rule enforcement, caller blast radius, and AST verification status.
func CalculateCompositeRankScore(
	severity string,
	isDrixyRule bool,
	callersCount int,
	astVerified bool,
	fileImportance float64,
) float64 {
	// Base severity weights
	baseWeight := 20.0
	switch strings.ToLower(severity) {
	case "critical":
		baseWeight = 85.0
	case "high":
		baseWeight = 65.0
	case "medium":
		baseWeight = 40.0
	case "low":
		baseWeight = 20.0
	}

	// Custom enterprise rule boost
	ruleBoost := 0.0
	if isDrixyRule {
		ruleBoost = 15.0
	}

	// Caller reachability blast radius log bonus (capped at 15.0)
	callerBonus := 0.0
	if callersCount > 0 {
		callerBonus = math.Min(15.0, math.Log2(float64(callersCount+1))*3.5)
	}

	// AST verification confidence multiplier (0.85 if unverified, 1.05 if syntax-verified)
	confMult := 0.85
	if astVerified {
		confMult = 1.05
	}

	// File importance scaling (0.8 to 1.2)
	fileMult := math.Max(0.8, math.Min(1.2, fileImportance))

	composite := (baseWeight + ruleBoost + callerBonus) * confMult * fileMult
	return math.Max(0.0, math.Min(100.0, composite))
}
