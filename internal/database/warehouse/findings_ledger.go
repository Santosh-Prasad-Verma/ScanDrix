package warehouse

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// FindingLifecycleState represents the operational status of a security finding.
type FindingLifecycleState string

const (
	StateDetected  FindingLifecycleState = "DETECTED"
	StateTriaged   FindingLifecycleState = "TRIAGED"
	StateDismissed FindingLifecycleState = "DISMISSED"
	StateResolved  FindingLifecycleState = "RESOLVED"
	StateRegressed FindingLifecycleState = "REGRESSED"
)

// DismissalReason enumerates compliant justifications for bypassing a finding.
type DismissalReason string

const (
	ReasonFalsePositive DismissalReason = "FALSE_POSITIVE"
	ReasonRiskAccepted  DismissalReason = "RISK_ACCEPTED"
	ReasonWillFixLater  DismissalReason = "WILL_FIX_LATER"
	ReasonNotApplicable DismissalReason = "NOT_APPLICABLE"
)

// TrackedFinding tracks the mutable state and full audit transitions of a detected flaw.
type TrackedFinding struct {
	Finding         models.CodeFinding    `json:"finding"`
	CurrentState    FindingLifecycleState `json:"current_state"`
	DismissalReason *DismissalReason      `json:"dismissal_reason,omitempty"`
	DismissalNote   string                `json:"dismissal_note,omitempty"`
	DismissedBy     string                `json:"dismissed_by,omitempty"`
	ResolvedInSHA   string                `json:"resolved_in_sha,omitempty"`
	DetectedAt      time.Time             `json:"detected_at"`
	TriagedAt       *time.Time            `json:"triaged_at,omitempty"`
	DismissedAt     *time.Time            `json:"dismissed_at,omitempty"`
	ResolvedAt      *time.Time            `json:"resolved_at,omitempty"`
	TimeOpen        time.Duration         `json:"time_open"`
}

// FindingsLedger manages state machines and event emission for all code review findings.
type FindingsLedger struct {
	mu       sync.RWMutex
	store    *EventStore
	findings map[uuid.UUID]*TrackedFinding
}

// NewFindingsLedger initializes the ledger backed by the event store.
func NewFindingsLedger(store *EventStore) *FindingsLedger {
	return &FindingsLedger{
		store:    store,
		findings: make(map[uuid.UUID]*TrackedFinding),
	}
}

// RecordDetection logs a newly discovered vulnerability into the ledger.
func (l *FindingsLedger) RecordDetection(ctx context.Context, f models.CodeFinding) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UTC()
	tf := &TrackedFinding{
		Finding:      f,
		CurrentState: StateDetected,
		DetectedAt:   now,
	}
	l.findings[f.ID] = tf

	evt, err := NewDomainEvent(f.WorkspaceID, f.ID, "FINDING", EventFindingDetected, map[string]any{
		"file_path": f.FilePath,
		"line":      f.StartLine,
		"severity":  f.Severity,
		"rule":      f.Title,
	}, "system")
	if err != nil {
		return err
	}

	return l.store.Append(ctx, evt)
}

// TriageFinding moves a finding from DETECTED to TRIAGED.
func (l *FindingsLedger) TriageFinding(ctx context.Context, findingID uuid.UUID, actorEmail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tf, ok := l.findings[findingID]
	if !ok {
		return fmt.Errorf("finding %s not found in ledger", findingID)
	}

	if tf.CurrentState != StateDetected {
		return fmt.Errorf("cannot triage finding in state %s", tf.CurrentState)
	}

	now := time.Now().UTC()
	tf.CurrentState = StateTriaged
	tf.TriagedAt = &now

	evt, err := NewDomainEvent(tf.Finding.WorkspaceID, findingID, "FINDING", EventFindingTriaged, map[string]any{
		"triaged_by": actorEmail,
	}, actorEmail)
	if err != nil {
		return err
	}

	return l.store.Append(ctx, evt)
}

// DismissFinding transitions a finding to DISMISSED with a validated audit reason.
func (l *FindingsLedger) DismissFinding(ctx context.Context, findingID uuid.UUID, reason DismissalReason, note, actorEmail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tf, ok := l.findings[findingID]
	if !ok {
		return fmt.Errorf("finding %s not found in ledger", findingID)
	}

	if tf.CurrentState == StateResolved {
		return errors.New("cannot dismiss an already resolved finding")
	}

	now := time.Now().UTC()
	tf.CurrentState = StateDismissed
	tf.DismissalReason = &reason
	tf.DismissalNote = note
	tf.DismissedBy = actorEmail
	tf.DismissedAt = &now
	tf.TimeOpen = now.Sub(tf.DetectedAt)

	evt, err := NewDomainEvent(tf.Finding.WorkspaceID, findingID, "FINDING", EventFindingDismissed, map[string]any{
		"reason":       reason,
		"note":         note,
		"dismissed_by": actorEmail,
	}, actorEmail)
	if err != nil {
		return err
	}

	return l.store.Append(ctx, evt)
}

// ResolveFinding marks a finding as fixed with the corresponding remediation commit.
func (l *FindingsLedger) ResolveFinding(ctx context.Context, findingID uuid.UUID, commitSHA, actorEmail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tf, ok := l.findings[findingID]
	if !ok {
		return fmt.Errorf("finding %s not found in ledger", findingID)
	}

	now := time.Now().UTC()
	tf.CurrentState = StateResolved
	tf.ResolvedInSHA = commitSHA
	tf.ResolvedAt = &now
	tf.TimeOpen = now.Sub(tf.DetectedAt)

	evt, err := NewDomainEvent(tf.Finding.WorkspaceID, findingID, "FINDING", EventFindingResolved, map[string]any{
		"resolved_in_sha": commitSHA,
		"resolved_by":     actorEmail,
	}, actorEmail)
	if err != nil {
		return err
	}

	return l.store.Append(ctx, evt)
}

// GetTrackedFinding returns the current state of a finding.
func (l *FindingsLedger) GetTrackedFinding(findingID uuid.UUID) (*TrackedFinding, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	tf, ok := l.findings[findingID]
	if !ok {
		return nil, false
	}
	cp := *tf
	return &cp, true
}

// CalculateMTTR calculates the Mean Time to Remediation for all resolved findings in a workspace.
func (l *FindingsLedger) CalculateMTTR(wsID uuid.UUID) time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var totalDuration time.Duration
	var count int

	for _, tf := range l.findings {
		if tf.Finding.WorkspaceID == wsID && tf.CurrentState == StateResolved && tf.ResolvedAt != nil {
			totalDuration += tf.TimeOpen
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return totalDuration / time.Duration(count)
}
