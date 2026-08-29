package warehouse

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// WorkspaceQualityMetrics summarizes aggregate security and quality health over a time window.
type WorkspaceQualityMetrics struct {
	WorkspaceID          uuid.UUID              `json:"workspace_id"`
	WindowStart          time.Time              `json:"window_start"`
	WindowEnd            time.Time              `json:"window_end"`
	TotalReviews         int                    `json:"total_reviews"`
	ApprovedReviews      int                    `json:"approved_reviews"`
	BlockedReviews       int                    `json:"blocked_reviews"`
	ApprovalRate         float64                `json:"approval_rate"`
	TotalFindings        int                    `json:"total_findings"`
	SeverityCounts       map[string]int         `json:"severity_counts"`
	CategoryCounts       map[string]int         `json:"category_counts"`
	DismissedCount       int                    `json:"dismissed_count"`
	ResolvedCount        int                    `json:"resolved_count"`
	MeanTimeToRemediate  time.Duration          `json:"mean_time_to_remediate"`
}

// MetricsAggregator queries the event store to derive actionable executive quality dashboards.
type MetricsAggregator struct {
	store  *EventStore
	ledger *FindingsLedger
}

// NewMetricsAggregator initializes the metrics aggregation engine.
func NewMetricsAggregator(store *EventStore, ledger *FindingsLedger) *MetricsAggregator {
	return &MetricsAggregator{
		store:  store,
		ledger: ledger,
	}
}

// Aggregate computes executive quality metrics across a defined period.
func (a *MetricsAggregator) Aggregate(ctx context.Context, wsID uuid.UUID, since, until time.Time) (*WorkspaceQualityMetrics, error) {
	events, err := a.store.QueryEvents(ctx, EventFilter{
		WorkspaceID: wsID,
		Since:       &since,
		Until:       &until,
	})
	if err != nil {
		return nil, err
	}

	metrics := &WorkspaceQualityMetrics{
		WorkspaceID:    wsID,
		WindowStart:    since,
		WindowEnd:      until,
		SeverityCounts: make(map[string]int),
		CategoryCounts: make(map[string]int),
	}

	for _, e := range events {
		switch e.EventType {
		case EventReviewTriggered:
			metrics.TotalReviews++
		case EventMergeApproved:
			metrics.ApprovedReviews++
		case EventMergeBlocked:
			metrics.BlockedReviews++
		case EventFindingDismissed:
			metrics.DismissedCount++
		case EventFindingResolved:
			metrics.ResolvedCount++
		case EventFindingDetected:
			metrics.TotalFindings++

			var payload struct {
				Severity models.FindingSeverity `json:"severity"`
				Category string                 `json:"category"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err == nil {
				if payload.Severity != "" {
					metrics.SeverityCounts[string(payload.Severity)]++
				}
				if payload.Category != "" {
					metrics.CategoryCounts[payload.Category]++
				}
			}
		}
	}

	if metrics.TotalReviews > 0 {
		metrics.ApprovalRate = float64(metrics.ApprovedReviews) / float64(metrics.TotalReviews)
	}

	if a.ledger != nil {
		metrics.MeanTimeToRemediate = a.ledger.CalculateMTTR(wsID)
	}

	return metrics, nil
}
