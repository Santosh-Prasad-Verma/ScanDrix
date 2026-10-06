package clireview

import (
	"errors"
	"sort"
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
		// Tenant scope. This was accepted in CliReviewsQuery and then never
		// applied, so the list would have returned every organization's
		// reviews to any caller. AUDIT_REMEDIATION.md F-15c.
		if q.OrganizationID != "" && r.OrganizationID != q.OrganizationID {
			continue
		}

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

	sort.Slice(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID < matched[j].ID
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

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

// GetCliReviewByID retrieves an individual review record by ID, scoped to the
// calling organization. A record belonging to another tenant is reported as
// not found rather than returned, so the ID cannot be used to probe other
// organizations. AUDIT_REMEDIATION.md F-15c.
func (ds *DashboardStore) GetCliReviewByID(id, organizationID string) (*CliReviewSummary, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	r, ok := ds.reviews[id]
	if !ok {
		return nil, errors.New("cli review not found")
	}
	if organizationID != "" && r.OrganizationID != organizationID {
		return nil, errors.New("cli review not found")
	}
	return &r, nil
}
