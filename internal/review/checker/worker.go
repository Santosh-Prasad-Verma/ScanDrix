package checker

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database/warehouse"
)

// SuggestionCheckWorker processes code updates to track developer suggestion adoption.
type SuggestionCheckWorker struct {
	mu       sync.RWMutex
	ledger   *warehouse.FindingsLedger
	verifier *SuggestionVerifier
	history  []SuggestionVerification
}

// NewSuggestionCheckWorker initializes the worker.
func NewSuggestionCheckWorker(ledger *warehouse.FindingsLedger) *SuggestionCheckWorker {
	return &SuggestionCheckWorker{
		ledger:   ledger,
		verifier: NewSuggestionVerifier(),
		history:  make([]SuggestionVerification, 0),
	}
}

// CheckCommit processes an updated code revision for an existing finding.
func (w *SuggestionCheckWorker) CheckCommit(
	ctx context.Context,
	findingID, wsID uuid.UUID,
	prNum int,
	originalCode, suggestedCode, committedCode string,
	regexRule string,
	commitSHA string,
) (*SuggestionVerification, error) {
	ver := w.verifier.Verify(findingID, wsID, prNum, originalCode, suggestedCode, committedCode, regexRule, commitSHA)

	w.mu.Lock()
	w.history = append(w.history, *ver)
	w.mu.Unlock()

	// If resolved, update the persistent findings ledger state machine
	if ver.Status == StatusAcceptedExact || ver.Status == StatusAcceptedManual {
		if w.ledger != nil {
			_ = w.ledger.ResolveFinding(ctx, findingID, commitSHA, "scandrix-suggestion-checker")
		}
	}

	return ver, nil
}

// ComputeAdoptionMetrics aggregates adoption statistics for a workspace.
func (w *SuggestionCheckWorker) ComputeAdoptionMetrics(wsID uuid.UUID) AdoptionMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()

	metrics := AdoptionMetrics{
		WorkspaceID: wsID,
	}

	for _, h := range w.history {
		if h.WorkspaceID != wsID {
			continue
		}

		metrics.TotalSuggestions++
		switch h.Status {
		case StatusAcceptedExact:
			metrics.AcceptedExactCount++
		case StatusAcceptedManual:
			metrics.AcceptedManualCount++
		case StatusRejected:
			metrics.RejectedCount++
		case StatusPending:
			metrics.PendingCount++
		}
	}

	if metrics.TotalSuggestions > 0 {
		accepted := float64(metrics.AcceptedExactCount + metrics.AcceptedManualCount)
		metrics.AdoptionRate = accepted / float64(metrics.TotalSuggestions)
	}

	return metrics
}
