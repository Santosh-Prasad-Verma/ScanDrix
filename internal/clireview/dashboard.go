package clireview

import (
	"errors"
	"strings"
	"sync"
)

// DashboardStore holds historical review records for queries.
type DashboardStore struct {
	mu      sync.RWMutex
	reviews map[string]CliReviewSummary
}

// NewDashboardStore creates an initialized dashboard store.
func NewDashboardStore() *DashboardStore {
	return &DashboardStore{
		reviews: make(map[string]CliReviewSummary),
	}
}

// RecordReview saves a review summary for dashboard reporting.
func (ds *DashboardStore) RecordReview(summary CliReviewSummary) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.reviews[summary.ID] = summary
}

// GetCliReviews executes filtered search against historical reviews.
func (ds *DashboardStore) GetCliReviews(q CliReviewsQuery) CliReviewsListResponse {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	var matched []CliReviewSummary
	for _, r := range ds.reviews {
		if q.Search != "" {
			if !strings.Contains(strings.ToLower(r.Summary), strings.ToLower(q.Search)) &&
				!strings.Contains(strings.ToLower(r.Branch), strings.ToLower(q.Search)) &&
				!strings.Contains(strings.ToLower(r.UserEmail), strings.ToLower(q.Search)) {
				continue
			}
		}

		if q.StartDate != nil && r.CreatedAt.Before(*q.StartDate) {
			continue
		}
		if q.EndDate != nil && r.CreatedAt.After(*q.EndDate) {
			continue
		}

		matched = append(matched, r)
	}

	total := len(matched)
	if q.Offset >= total {
		return CliReviewsListResponse{Items: nil, Total: total}
	}

	end := total
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}

	return CliReviewsListResponse{
		Items: matched[q.Offset:end],
		Total: total,
	}
}

// GetCliReviewByID retrieves an individual review record by ID.
func (ds *DashboardStore) GetCliReviewByID(id string) (*CliReviewSummary, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	r, ok := ds.reviews[id]
	if !ok {
		return nil, errors.New("cli review not found")
	}
	return &r, nil
}
